//go:build installer

package main

import "unsafe"

func showMessage(message, title *uint16) {
	messageBox.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}
