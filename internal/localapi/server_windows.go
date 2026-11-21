//go:build windows

package localapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"food-truck-printer/internal/discovery"
)

const DefaultAddr = "127.0.0.1:17890"

func Serve(ctx context.Context, addr string, simulation bool, connected func() bool) error {
	if addr == "" {
		addr = DefaultAddr
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/health" || r.URL.Path == "/api/health" {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true, "service": "food-truck-printer", "time": time.Now().UTC(),
				"connected": connected(), "simulation": simulation,
			})
			return
		}
		if r.URL.Path == "/printers/discover" || r.URL.Path == "/api/printers/discover" {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Network string `json:"network"`
				Port    int    `json:"port"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "envía JSON con network, por ejemplo 192.168.101.x"})
				return
			}
			if request.Network == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "network es obligatorio"})
				return
			}
			scanCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			result, err := discovery.Discover(scanCtx, request.Network, request.Port)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		if r.URL.Path == "/printers/check" || r.URL.Path == "/api/printers/check" {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			host := r.URL.Query().Get("host")
			if host == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indica host"})
				return
			}
			port, _ := strconv.Atoi(r.URL.Query().Get("port"))
			printer, err := discovery.Check(r.Context(), host, port)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "host": host, "port": port, "error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "printer": printer})
			return
		}
		http.NotFound(w, r)
	})
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("API local de impresoras en http://%s", addr)
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
