package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"food-truck-printer/internal/worker"
)

func main() {
	baseURL := strings.TrimRight(os.Getenv("PRINTER_API_URL"), "/")
	token := os.Getenv("PRINTER_AGENT_TOKEN")
	if baseURL == "" || token == "" {
		log.Fatal("Configura PRINTER_API_URL y PRINTER_AGENT_TOKEN")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client := worker.NewClient(baseURL, token)
	outputDir := os.Getenv("PRINTER_OUTPUT_DIR")
	for ctx.Err() == nil {
		var response struct{ Job *worker.Job }
		err := client.Post(ctx, "/printer-worker/claim", struct{}{}, &response)
		if err != nil {
			log.Print(err)
		}
		if response.Job == nil {
			wait(ctx)
			continue
		}
		job := response.Job
		safe, err := job.Print(ctx, outputDir)
		result := map[string]any{"attempt": job.Attempt, "status": "PRINTED"}
		if err != nil {
			message := []rune(err.Error())
			if len(message) > 500 {
				message = message[:500]
			}
			result["status"], result["error"], result["safeToRetry"] = "FAILED", string(message), safe
		}
		// Retry delivery of the result, never the physical print.
		ackCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		for ackCtx.Err() == nil {
			var ack struct{ OK bool }
			if err := client.Post(ackCtx, "/printer-worker/jobs/"+job.ID+"/result", result, &ack); err == nil && ack.OK {
				break
			}
			log.Printf("Confirmación pendiente para %s", job.ID)
			wait(ackCtx)
		}
		if ackCtx.Err() != nil {
			log.Printf("Resultado sin confirmar para %s", job.ID)
		}
		cancel()
	}
}

func wait(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
	}
}
