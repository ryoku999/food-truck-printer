package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"food-truck-printer/internal/config"
	"food-truck-printer/internal/localapi"
	"food-truck-printer/internal/realtime"
	"food-truck-printer/internal/worker"
	"golang.org/x/sys/windows/svc"
)

type serviceHandler struct{}

func (serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	dir, err := config.InstallDir()
	if err != nil {
		return true, 1
	}
	data := filepath.Join(dir, "data")
	logFile := filepath.Join(data, "agent.log")
	if info, err := os.Stat(logFile); err == nil && info.Size() > 2*1024*1024 {
		_ = os.Rename(logFile, logFile+".previous")
	}
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return true, 2
	}
	defer file.Close()
	log.SetOutput(file)
	settings, token, err := config.Load(filepath.Join(data, "config.json"))
	if err != nil {
		log.Print(err)
		return true, 3
	}
	output := ""
	if settings.Simulation {
		output = filepath.Join(data, "out")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := localapi.Serve(ctx, ""); err != nil {
			log.Printf("API local detenida: %v", err)
		}
	}()
	wake := realtime.NewWakeupClient(ctx, settings.URL, token)
	result := make(chan error, 1)
	go func() {
		result <- run(ctx, worker.NewClient(settings.URL, token), output, filepath.Join(data, "state"), wake)
	}()
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	statuses <- current
	log.Print("Agente iniciado como servicio")
	for {
		select {
		case err := <-result:
			if err != nil {
				log.Print(err)
				return true, 4
			}
			return false, 0
		case request, ok := <-requests:
			if !ok {
				return false, 0
			}
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- current
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending, WaitHint: 25000, CheckPoint: 1}
				cancel()
				select {
				case err := <-result:
					if err != nil {
						log.Print(err)
					}
				case <-time.After(20 * time.Second):
					log.Print("Tiempo de cierre agotado; el diario se recuperará al reiniciar")
				}
				return false, 0
			}
		}
	}
}
