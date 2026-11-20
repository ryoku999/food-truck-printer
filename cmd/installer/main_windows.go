//go:build installer

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"food-truck-printer/internal/config"
	"golang.org/x/sys/windows"
)

var messageBox = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

func main() {
	if !windows.GetCurrentProcessToken().IsElevated() {
		exe, err := os.Executable()
		if err == nil {
			err = shell("runas", exe)
		}
		if err != nil {
			showError(err)
		}
		return
	}
	if err := setup(); err != nil {
		showError(err)
	}
}

func shell(verb, target string) error {
	action, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, action, file, nil, nil, windows.SW_SHOWNORMAL)
}

func showError(err error) {
	title, _ := windows.UTF16PtrFromString("Food Truck Printer")
	message, _ := windows.UTF16PtrFromString(err.Error())
	showMessage(message, title)
}

func setup() error {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	key := hex.EncodeToString(secret[:])
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	host := listener.Addr().String()
	origin := "http://" + host
	endpoint := "/setup/" + key
	type view struct {
		APIURL, Error              string
		Existing, Simulation, Done bool
	}
	initial := view{APIURL: "http://localhost:8000/api"}
	if dir, err := config.InstallDir(); err == nil {
		if previous, _, err := config.Load(filepath.Join(dir, "data", "config.json")); err == nil {
			initial.APIURL, initial.Simulation, initial.Existing = previous.URL, previous.Simulation, true
		}
	}
	done := make(chan struct{})
	var lock sync.Mutex
	completed := false
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 90 * time.Second,
		IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != host || subtle.ConstantTimeCompare([]byte(r.URL.Path), []byte(endpoint)) != 1 {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			page := initial
			switch r.Method {
			case http.MethodGet:
				// Render the initial form.
			case http.MethodPost:
				if r.Header.Get("Origin") != origin {
					http.Error(w, "Origen inválido", http.StatusForbidden)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, 8192)
				if err := r.ParseForm(); err != nil {
					http.Error(w, "Formulario inválido", http.StatusBadRequest)
					return
				}
				lock.Lock()
				defer lock.Unlock()
				if completed {
					http.Error(w, "Instalación ya completada", http.StatusConflict)
					return
				}
				page.APIURL = r.PostForm.Get("apiUrl")
				page.Simulation = r.PostForm.Get("simulation") == "1"
				token := r.PostForm.Get("token")
				settings, err := prepare(page.APIURL, token, page.Simulation)
				if err == nil {
					err = install(settings)
				}
				if err != nil {
					page.Error = err.Error()
				} else {
					completed, page.Done = true, true
					defer close(done)
				}
			default:
				w.Header().Set("Allow", "GET, POST")
				http.Error(w, "Método inválido", http.StatusMethodNotAllowed)
				return
			}
			_ = setupPage.Execute(w, page)
		}),
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	defer server.Close()
	if err := shell("open", origin+endpoint); err != nil {
		return err
	}
	select {
	case <-done:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-stopped:
		return err
	case <-time.After(15 * time.Minute):
		return fmt.Errorf("la configuración local expiró; abre el instalador nuevamente")
	}
}

func prepare(apiURL, token string, simulation bool) (config.Settings, error) {
	if token != "" {
		return config.New(apiURL, token, simulation)
	}
	dir, err := config.InstallDir()
	if err != nil {
		return config.Settings{}, err
	}
	previous, _, err := config.Load(filepath.Join(dir, "data", "config.json"))
	if err != nil {
		return config.Settings{}, fmt.Errorf("ingresa el token del agente")
	}
	normalized, err := config.NormalizeURL(apiURL)
	if err != nil {
		return config.Settings{}, err
	}
	if normalized != previous.URL {
		return config.Settings{}, fmt.Errorf("ingresa el token de la nueva API")
	}
	previous.Simulation = simulation
	return previous, nil
}

var setupPage = template.Must(template.New("setup").Parse(`<!doctype html>
<html lang="es"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Food Truck Printer</title><style>
body{font:16px system-ui;margin:3rem auto;max-width:38rem;padding:0 1rem;color-scheme:light dark}
label{display:block;margin:1rem 0}input:not([type=checkbox]){display:block;box-sizing:border-box;width:100%;padding:.55rem;margin-top:.4rem}
button{font:inherit;padding:.55rem 1rem} [role=alert]{border:1px solid;padding:1rem;overflow-wrap:anywhere}
</style><main><h1>Food Truck Printer</h1>
{{if .Done}}<p>Instalación completada. El servicio quedó iniciado y arrancará automáticamente con Windows.</p>
<p>Puedes cerrar esta pestaña. En la app web, consulta el agente en <strong>Impresión</strong>.</p>
{{else}}
<p>Configura el agente que recoge los tickets de NestJS. El servicio se instalará en esta PC.</p>
{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
<form method="post" autocomplete="off">
<label>URL de la API<input name="apiUrl" type="url" value="{{.APIURL}}" required maxlength="2048"></label>
<label>Token del agente<input name="token" type="password" autocomplete="new-password" maxlength="64" {{if not .Existing}}required{{end}}></label>
{{if .Existing}}<p>Deja el token vacío para conservarlo si la URL es la misma.</p>{{end}}
<label><input type="checkbox" name="simulation" value="1" {{if .Simulation}}checked{{end}}> Guardar tickets como TXT</label>
<p>Para simular, usa un agente dedicado. Sus trabajos se confirmarán al escribir el archivo, sin imprimir en papel.</p>
<button>Instalar o actualizar servicio</button></form>
{{end}}</main></html>`))
