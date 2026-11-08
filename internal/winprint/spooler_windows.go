package winprint

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	spooler      = syscall.NewLazyDLL("winspool.drv")
	openPrinter  = spooler.NewProc("OpenPrinterW")
	closePrinter = spooler.NewProc("ClosePrinter")
	startDoc     = spooler.NewProc("StartDocPrinterW")
	endDoc       = spooler.NewProc("EndDocPrinter")
	startPage    = spooler.NewProc("StartPagePrinter")
	endPage      = spooler.NewProc("EndPagePrinter")
	writePrinter = spooler.NewProc("WritePrinter")
	abortPrinter = spooler.NewProc("AbortPrinter")
)

type documentInfo struct {
	name, output, dataType *uint16
}

func spoolError(operation string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s falló", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// SendRaw returns whether a failed job can be retried without duplicating output.
// Winspool calls are synchronous; context is checked between native calls.
func SendRaw(ctx context.Context, name string, data []byte) (safe bool, err error) {
	name = strings.TrimSpace(name)
	if name == "" || len(data) == 0 || uint64(len(data)) > uint64(^uint32(0)) {
		return true, fmt.Errorf("cola o contenido RAW inválido")
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	queue, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return true, err
	}
	title, _ := syscall.UTF16PtrFromString("Food Truck")
	raw, _ := syscall.UTF16PtrFromString("RAW")
	var handle syscall.Handle
	if ok, _, callErr := openPrinter.Call(uintptr(unsafe.Pointer(queue)), uintptr(unsafe.Pointer(&handle)), 0); ok == 0 {
		return true, spoolError("OpenPrinterW", callErr)
	}
	defer closePrinter.Call(uintptr(handle))
	if err := ctx.Err(); err != nil {
		return true, err
	}
	info := documentInfo{name: title, dataType: raw}
	if id, _, callErr := startDoc.Call(uintptr(handle), 1, uintptr(unsafe.Pointer(&info))); id == 0 {
		return true, spoolError("StartDocPrinterW", callErr)
	}
	finished := false
	defer func() {
		if !finished {
			// Best effort only; never infer that bytes already submitted were not printed.
			abortPrinter.Call(uintptr(handle))
		}
	}()
	if ok, _, callErr := startPage.Call(uintptr(handle)); ok == 0 {
		return true, spoolError("StartPagePrinter", callErr)
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	var written uint32
	if ok, _, callErr := writePrinter.Call(uintptr(handle), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written))); ok == 0 {
		return false, spoolError("WritePrinter", callErr)
	}
	if int(written) != len(data) {
		return false, fmt.Errorf("WritePrinter escribió %d de %d bytes", written, len(data))
	}
	if ok, _, callErr := endPage.Call(uintptr(handle)); ok == 0 {
		return false, spoolError("EndPagePrinter", callErr)
	}
	if ok, _, callErr := endDoc.Call(uintptr(handle)); ok == 0 {
		return false, spoolError("EndDocPrinter", callErr)
	}
	finished = true
	return false, nil
}
