//go:build windows

package discovery

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	defaultPort      = 9100
	maxScanAddresses = 1024
	maxScanWorkers   = 128
	scanTimeout      = 2 * time.Second
)

type NetworkPrinter struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	ConnectionType string `json:"connectionType"`
	LatencyMS      int64  `json:"latencyMs"`
}

type SystemPrinter struct {
	Name       string `json:"name"`
	PortName   string `json:"portName"`
	DriverName string `json:"driverName,omitempty"`
	Status     uint32 `json:"status"`
}

type Result struct {
	Network          []NetworkPrinter `json:"network"`
	System           []SystemPrinter  `json:"system"`
	RequestedNetwork string           `json:"requestedNetwork"`
	Port             int              `json:"port"`
	ScannedAt        time.Time        `json:"scannedAt"`
}

func Check(ctx context.Context, host string, port int) (NetworkPrinter, error) {
	if port < 1 || port > 65535 {
		port = defaultPort
	}
	started := time.Now()
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return NetworkPrinter{}, err
	}
	_ = conn.Close()
	return NetworkPrinter{Host: host, Port: port, ConnectionType: "NETWORK_TCP", LatencyMS: time.Since(started).Milliseconds()}, nil
}

func Discover(ctx context.Context, requested string, port int) (Result, error) {
	if port < 1 || port > 65535 {
		port = defaultPort
	}
	network, err := parseRequestedNetwork(requested)
	if err != nil {
		return Result{}, err
	}
	result := Result{Network: []NetworkPrinter{}, System: []SystemPrinter{}, RequestedNetwork: network.String(), Port: port, ScannedAt: time.Now().UTC()}
	result.System = enumSystemPrinters()

	targets := hosts(network)
	if len(targets) > maxScanAddresses {
		return Result{}, fmt.Errorf("la red solicitada es demasiado grande; usa una red entre /16 y /24")
	}
	type hit struct{ printer NetworkPrinter }
	jobs := make(chan string)
	hits := make(chan hit, len(targets))
	var wg sync.WaitGroup
	workers := maxScanWorkers
	if len(targets) < workers {
		workers = len(targets)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range jobs {
				started := time.Now()
				conn, err := (&net.Dialer{Timeout: scanTimeout}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
				if err != nil {
					continue
				}
				_ = conn.Close()
				hits <- hit{printer: NetworkPrinter{Host: host, Port: port, ConnectionType: "NETWORK_TCP", LatencyMS: time.Since(started).Milliseconds()}}
			}
		}()
	}
queueTargets:
	for _, target := range targets {
		select {
		case jobs <- target.String():
		case <-ctx.Done():
			break queueTargets
		}
	}
	close(jobs)
	wg.Wait()
	close(hits)
	for item := range hits {
		result.Network = append(result.Network, item.printer)
	}
	return result, nil
}

func parseRequestedNetwork(value string) (*net.IPNet, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, ".x") || strings.HasSuffix(value, ".X") {
		value = strings.TrimSuffix(strings.TrimSuffix(value, ".x"), ".X") + ".0/24"
	}
	ip, network, err := net.ParseCIDR(value)
	if err != nil || ip.To4() == nil {
		return nil, fmt.Errorf("red inválida; usa 192.168.101.x o 192.168.101.0/24")
	}
	network.IP = network.IP.To4()
	prefix, bits := network.Mask.Size()
	if bits != 32 || prefix < 16 || prefix > 30 {
		return nil, fmt.Errorf("la red debe ser IPv4 entre /16 y /30")
	}
	return network, nil
}

func hosts(network *net.IPNet) []net.IP {
	base := binary.BigEndian.Uint32(network.IP.To4())
	bits, totalBits := network.Mask.Size()
	if totalBits != 32 {
		return nil
	}
	total := 1 << uint(32-bits)
	first, last := 0, total
	if total > 2 {
		first, last = 1, total-1
	}
	result := make([]net.IP, 0, last-first)
	for offset := first; offset < last; offset++ {
		value := make(net.IP, net.IPv4len)
		binary.BigEndian.PutUint32(value, base+uint32(offset))
		ip := value
		result = append(result, ip)
	}
	return result
}

var (
	spooler       = syscall.NewLazyDLL("winspool.drv")
	enumPrintersW = spooler.NewProc("EnumPrintersW")
)

type printerInfo2 struct {
	ServerName      *uint16
	PrinterName     *uint16
	ShareName       *uint16
	PortName        *uint16
	DriverName      *uint16
	Comment         *uint16
	Location        *uint16
	DevMode         uintptr
	SepFile         *uint16
	PrintProcessor  *uint16
	Datatype        *uint16
	Parameters      *uint16
	Security        uintptr
	Attributes      uint32
	Priority        uint32
	DefaultPriority uint32
	StartTime       uint32
	UntilTime       uint32
	Status          uint32
	Jobs            uint32
	AveragePPM      uint32
}

func enumSystemPrinters() []SystemPrinter {
	const flags = 0x00000002 | 0x00000004
	var needed, returned uint32
	_, _, _ = enumPrintersW.Call(flags, 0, 2, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil
	}
	buffer := make([]byte, needed)
	if ok, _, _ := enumPrintersW.Call(flags, 0, 2, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned))); ok == 0 {
		return nil
	}
	items := make([]SystemPrinter, 0, returned)
	size := unsafe.Sizeof(printerInfo2{})
	for i := uint32(0); i < returned; i++ {
		info := (*printerInfo2)(unsafe.Pointer(uintptr(unsafe.Pointer(&buffer[0])) + uintptr(i)*size))
		name := utf16String(info.PrinterName)
		if strings.TrimSpace(name) == "" {
			continue
		}
		items = append(items, SystemPrinter{Name: name, PortName: utf16String(info.PortName), DriverName: utf16String(info.DriverName), Status: info.Status})
	}
	return items
}

func utf16String(value *uint16) string {
	if value == nil {
		return ""
	}
	return windows.UTF16PtrToString(value)
}
