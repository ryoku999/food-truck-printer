package worker

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"food-truck-printer/internal/escpos"
	"food-truck-printer/internal/winprint"
)

type Job struct {
	ID      string
	Attempt int
	Payload struct {
		Lines          []string
		OrderNumber    int
		ProductionArea string
		Items          []struct {
			ProductNameSnapshot string
			Quantity            int
			Notes               string
		}
	}
	Printer struct {
		ConnectionType string
		Host           string
		Port           int
		SystemName     string
		PaperWidth     int
	}
}

// Print reports whether a failure occurred before any delivery was attempted.
func (j Job) Print(ctx context.Context, outputDir string) (bool, error) {
	if len(j.Payload.Lines) == 0 && len(j.Payload.Items) > 0 {
		j.Payload.Lines = []string{fmt.Sprintf("%s | Pedido #%d", j.Payload.ProductionArea, j.Payload.OrderNumber)}
		for _, item := range j.Payload.Items {
			j.Payload.Lines = append(j.Payload.Lines, fmt.Sprintf("%d x %s", item.Quantity, item.ProductNameSnapshot))
			if item.Notes != "" {
				j.Payload.Lines = append(j.Payload.Lines, "  "+item.Notes)
			}
		}
	}
	lines := escpos.Lines(j.Payload.Lines, j.Printer.PaperWidth)
	if len(lines) == 0 {
		return true, fmt.Errorf("trabajo sin contenido")
	}
	if outputDir != "" {
		filename := j.ID + ".txt"
		if !filepath.IsLocal(filename) || filepath.Base(filename) != filename {
			return true, fmt.Errorf("identificador inválido para salida TXT")
		}
		if err := os.MkdirAll(outputDir, 0700); err != nil {
			return true, err
		}
		content := strings.Join(lines, "\n") + "\n\n\n"
		return true, os.WriteFile(filepath.Join(outputDir, filename), []byte(content), 0600)
	}
	data, err := escpos.Encode(lines)
	if err != nil {
		return true, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	switch j.Printer.ConnectionType {
	case "NETWORK_TCP":
		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(j.Printer.Host, strconv.Itoa(j.Printer.Port)))
		if err != nil {
			return true, err
		}
		defer conn.Close()
		deadline, _ := ctx.Deadline()
		if err = conn.SetWriteDeadline(deadline); err != nil {
			return true, err
		}
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		n, err := conn.Write(data)
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
		return false, err
	case "SYSTEM_PRINTER":
		return winprint.SendRaw(ctx, j.Printer.SystemName, data)
	default:
		return true, fmt.Errorf("tipo de conexión incompatible")
	}
}
