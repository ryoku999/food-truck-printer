//go:build installer

package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"food-truck-printer/internal/config"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

//go:embed payload/agent.exe
var agent []byte

func install(settings config.Settings) error {
	dir, err := config.InstallDir()
	if err != nil {
		return err
	}
	if err := config.PrivateDir(dir); err != nil {
		return err
	}
	lockName, err := windows.UTF16PtrFromString(filepath.Join(dir, "setup.lock"))
	if err != nil {
		return err
	}
	lock, err := windows.CreateFile(lockName, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return fmt.Errorf("no se pudo bloquear la instalación: %w", err)
	}
	defer windows.CloseHandle(lock)
	data := filepath.Join(dir, "data")
	if err := config.PrivateDir(data); err != nil {
		return err
	}
	configPath := filepath.Join(data, "config.json")
	exePath := filepath.Join(dir, "agent.exe")
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(config.ServiceName)
	existing := err == nil
	if err != nil && !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	var oldExe, oldConfig []byte
	wasRunning := false
	if existing {
		defer service.Close()
		current, err := service.Config()
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.Trim(current.BinaryPathName, "\""), exePath) {
			return fmt.Errorf("ya existe un servicio con este nombre en otra ruta")
		}
		status, err := service.Query()
		if err != nil {
			return err
		}
		wasRunning = status.State != svc.Stopped
		oldExe, err = os.ReadFile(exePath)
		if err != nil {
			return err
		}
		oldConfig, err = os.ReadFile(configPath)
		if err != nil {
			return err
		}
		if err := stop(service); err != nil {
			return err
		}
	}
	restore := func(cause error) error {
		if !existing {
			return cause
		}
		if err := stop(service); err != nil {
			return errors.Join(cause, err)
		}
		err := errors.Join(config.AtomicWrite(exePath, oldExe), config.AtomicWrite(configPath, oldConfig))
		if err == nil && wasRunning {
			err = service.Start()
		}
		return errors.Join(cause, err)
	}
	if existing {
		var previous config.Settings
		if err := json.Unmarshal(oldConfig, &previous); err == nil && previous.URL != settings.URL {
			key := sha256.Sum256([]byte(previous.URL))
			pending := filepath.Join(data, "state", fmt.Sprintf("%x", key[:]), "pending.json")
			if _, err := os.Stat(pending); err == nil || !errors.Is(err, os.ErrNotExist) {
				if wasRunning {
					err := service.Start()
					if err != nil {
						return err
					}
				}
				return fmt.Errorf("resuelve la confirmación pendiente con la API anterior antes de cambiar la URL")
			}
		}
	}
	if err := config.AtomicWrite(exePath, agent); err != nil {
		return restore(err)
	}
	if err := config.AtomicWrite(configPath, encoded); err != nil {
		return restore(err)
	}
	if !existing {
		service, err = manager.CreateService(config.ServiceName, exePath, mgr.Config{
			DisplayName: "Food Truck Printer", Description: "Agente de impresión del restaurante",
			StartType: mgr.StartAutomatic, DelayedAutoStart: true, Dependencies: []string{"Spooler"},
		})
		if err != nil {
			return err
		}
		defer service.Close()
	}
	configure := func() error {
		if err := service.SetRecoveryActions([]mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
			{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		}, 86400); err != nil {
			return err
		}
		if err := service.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
			return err
		}
		if err := service.Start(); err != nil {
			return err
		}
		return waitFor(service, svc.Running)
	}
	if err := configure(); err != nil {
		if existing {
			return restore(err)
		}
		_ = stop(service)
		return errors.Join(err, service.Delete())
	}
	return nil
}

func stop(service *mgr.Service) error {
	status, err := service.Query()
	if err != nil || status.State == svc.Stopped {
		return err
	}
	if status.State != svc.StopPending {
		if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return err
		}
	}
	return waitFor(service, svc.Stopped)
}

func waitFor(service *mgr.Service, target svc.State) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == target {
			return nil
		}
		if target == svc.Running && status.State == svc.Stopped {
			return fmt.Errorf("el servicio se detuvo al arrancar; revisa data\\agent.log")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("tiempo de espera agotado al cambiar el estado del servicio")
}
