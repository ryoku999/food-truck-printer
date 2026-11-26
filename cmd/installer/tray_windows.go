package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"food-truck-printer/internal/config"
	"golang.org/x/sys/windows/registry"
)

var tray []byte

func installTray() error {
	dir, err := config.TrayDir()
	if err != nil {
		return err
	}
	// A new version can be installed while the old icon is still running.
	digest := sha256.Sum256(tray)
	path := filepath.Join(dir, fmt.Sprintf("tray-%x.exe", digest[:8]))
	if current, err := os.ReadFile(path); err != nil || sha256.Sum256(current) != digest {
		if err := config.AtomicWrite(path, tray); err != nil {
			return err
		}
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("FoodTruckPrinterStatus", `"`+path+`"`); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "tray-????????????????.exe"))
	for _, file := range old {
		if file != path {
			_ = os.Remove(file)
		}
	}
	return nil
}
