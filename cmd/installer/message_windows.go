package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var messageBox = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

func showMessage(message, title *uint16) {
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}
