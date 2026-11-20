//go:build windows

package realtime

import (
	"context"
	"log"
	"net/url"
	"strings"
	"time"

	socketio "github.com/arcaela/socket.io-client-go"
)

// NewWakeupClient connects to the printer-worker Socket.IO namespace. The
// returned channel is only a hint to wake the HTTP claim loop; it never carries
// the print job itself.
func NewWakeupClient(ctx context.Context, apiURL, token string) <-chan struct{} {
	wake := make(chan struct{}, 1)
	base, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil || base.Host == "" {
		log.Printf("Socket.IO de impresión deshabilitado: URL inválida")
		return wake
	}
	base.Path = ""
	base.RawQuery = ""
	base.Fragment = ""
	switch base.Scheme {
	case "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	default:
		log.Printf("Socket.IO de impresión deshabilitado: esquema de API no compatible")
		return wake
	}
	options := socketio.Options{
		Auth:              map[string]interface{}{"token": token},
		ReconnectAttempts: -1,
		ReconnectDelay:    time.Second,
		ReconnectDelayMax: 15 * time.Second,
		Timeout:           10 * time.Second,
	}
	socket := socketio.New(base.String(), options)
	namespace := socket.Of("/printer-worker")
	namespace.OnConnect(func() { log.Printf("Socket.IO de impresión conectado") })
	namespace.On("printer.jobs.available", func(...interface{}) {
		select {
		case wake <- struct{}{}:
		default:
		}
	})
	namespace.OnDisconnect(func(reason string) { log.Printf("Socket.IO de impresión desconectado: %s", reason) })
	namespace.OnError(func(err error) { log.Printf("Socket.IO de impresión: %v", err) })
	go func() {
		<-ctx.Done()
		_ = socket.Close()
	}()
	return wake
}
