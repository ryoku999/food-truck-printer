package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"food-truck-printer/internal/config"
	"food-truck-printer/internal/localapi"
	"food-truck-printer/internal/realtime"
	"food-truck-printer/internal/worker"
	"golang.org/x/sys/windows/svc"
)

func main() {
	loadDotEnv(filepath.Join(".", ".env"))
	service, err := svc.IsWindowsService()
	if err != nil {
		log.Fatal(err)
	}
	if service {
		if err := svc.Run(config.ServiceName, serviceHandler{}); err != nil {
			log.Fatal(err)
		}
		return
	}
	baseURL := strings.TrimRight(os.Getenv("PRINTER_API_URL"), "/")
	token := os.Getenv("PRINTER_AGENT_TOKEN")
	if baseURL == "" || token == "" {
		log.Fatal("Configura PRINTER_API_URL y PRINTER_AGENT_TOKEN")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	client := worker.NewClient(baseURL, token)
	output := os.Getenv("PRINTER_OUTPUT_DIR")
	go func() {
		if err := localapi.Serve(ctx, os.Getenv("PRINTER_LOCAL_ADDR"), output != "", client.Connected); err != nil {
			log.Printf("API local detenida: %v", err)
		}
	}()
	wake := realtime.NewWakeupClient(ctx, baseURL, token)
	err = run(ctx, client, output, os.Getenv("PRINTER_STATE_DIR"), wake)
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// loadDotEnv is intentionally limited to interactive development. Installed
// services use the protected config.json created by the installer.
func loadDotEnv(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if key != "" {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, value)
			}
		}
	}
}

func run(ctx context.Context, client *worker.Client, outputDir, stateDir string, wake <-chan struct{}) error {
	journal, err := worker.OpenJournal(stateDir, client.BaseURL)
	if err != nil {
		return err
	}
	defer journal.Close()
	pending, err := journal.Load()
	if err != nil {
		return err
	}
	if pending != nil && pending.Result == nil {
		pending.Result = &worker.Result{
			Attempt: pending.Attempt, Status: "FAILED",
			Error: "El agente se interrumpió durante el envío. Revisa la salida antes de generar otro trabajo.",
		}
		if err := journal.Save(*pending); err != nil {
			return err
		}
		log.Printf("Trabajo %s marcado para revisión; no se volverá a imprimir", pending.JobID)
	}
	for ctx.Err() == nil {
		if pending != nil {
			if err := confirm(ctx, client, journal, *pending, wake); err != nil {
				return err
			}
			if ctx.Err() != nil {
				break
			}
			pending = nil
			continue
		}
		var response struct{ Job *worker.Job }
		err := client.Post(ctx, "/printer-worker/claim", struct{}{}, &response)
		if err != nil {
			log.Print(err)
			waitOrWake(ctx, wake, 3*time.Second)
			continue
		}
		if response.Job == nil {
			waitOrWake(ctx, wake, 60*time.Second)
			continue
		}
		job := response.Job
		if ctx.Err() != nil {
			// A claimed job still needs a durable record even when shutdown starts.
			log.Printf("Deteniendo el agente tras reservar %s", job.ID)
		}
		pending = &worker.Pending{JobID: job.ID, Attempt: job.Attempt}
		if err := journal.Save(*pending); err != nil {
			return fmt.Errorf("no se imprimió el trabajo %s: %w", job.ID, err)
		}
		safe, err := job.Print(ctx, outputDir)
		result := worker.Result{Attempt: job.Attempt, Status: "PRINTED"}
		if err != nil {
			message := []rune(err.Error())
			if len(message) > 500 {
				message = message[:500]
			}
			result.Status, result.Error, result.SafeToRetry = "FAILED", string(message), safe
		}
		pending.Result = &result
		if err := journal.Save(*pending); err != nil {
			return fmt.Errorf("no se pudo guardar el resultado de %s; requiere revisión: %w", job.ID, err)
		}
	}
	return nil
}

func confirm(ctx context.Context, client *worker.Client, journal *worker.Journal, pending worker.Pending, wake <-chan struct{}) error {
	for ctx.Err() == nil {
		var ack struct{ OK bool }
		err := client.Post(ctx, "/printer-worker/jobs/"+pending.JobID+"/result", pending.Result, &ack)
		if err == nil && ack.OK {
			return journal.Clear()
		}
		var status *worker.HTTPError
		if errors.As(err, &status) && status.Status == http.StatusConflict {
			// The server says this attempt is no longer current. Preserve the evidence.
			if err := journal.Archive(pending); err != nil {
				return err
			}
			log.Printf("Confirmación rechazada para %s, intento %d; guardada para revisión", pending.JobID, pending.Attempt)
			return nil
		}
		if err == nil {
			err = fmt.Errorf("la API no confirmó el resultado")
		}
		log.Printf("Confirmación pendiente para %s: %v", pending.JobID, err)
		waitOrWake(ctx, wake, 3*time.Second)
	}
	return nil
}

func wait(ctx context.Context) {
	waitOrWake(ctx, nil, 3*time.Second)
}

func waitOrWake(ctx context.Context, wake <-chan struct{}, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	case <-wake:
	}
}
