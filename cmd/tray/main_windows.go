package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"food-truck-printer/internal/localapi"
	"github.com/getlantern/systray"
	"golang.org/x/sys/windows"
)

//go:embed printer.ico
var printerIcon []byte

const appName = "Food Truck Printer"

func main() {
	name, err := windows.UTF16PtrFromString(`Local\FoodTruckPrinterStatus`)
	if err != nil {
		return
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if handle != 0 {
		defer windows.CloseHandle(handle)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || err != nil {
		return
	}
	systray.Run(onReady, func() {})
}

func onReady() {
	systray.SetIcon(printerIcon)
	systray.SetTooltip(appName)
	statusItem := systray.AddMenuItem("Consultando agente…", "")
	statusItem.Disable()
	systray.AddSeparator()
	closeItem := systray.AddMenuItem("Cerrar icono", "El servicio de impresión seguirá ejecutándose")

	go func() {
		client := &http.Client{
			Timeout: time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		var previous string
		for {
			current := printerStatus(client)
			if current != previous {
				statusItem.SetTitle(current)
				systray.SetTooltip(appName + " · " + current)
				previous = current
			}
			select {
			case <-ticker.C:
			case <-closeItem.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func printerStatus(client *http.Client) string {
	const unavailable = "Servicio local no disponible"
	response, err := client.Get("http://" + localapi.DefaultAddr + "/health")
	if err != nil {
		return unavailable
	}
	defer response.Body.Close()
	var status struct {
		OK         bool
		Service    string
		Connected  *bool
		Simulation bool
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&status) != nil ||
		!status.OK || status.Service != "food-truck-printer" {
		return unavailable
	}
	if status.Connected == nil {
		return "Servicio activo · estado de API no disponible"
	}
	connection := "Sin conexión a la API"
	if *status.Connected {
		connection = "Conectado a la API"
	}
	if status.Simulation {
		return "Simulación (TXT) · " + connection
	}
	return connection
}
