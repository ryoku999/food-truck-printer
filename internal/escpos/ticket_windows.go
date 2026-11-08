package escpos

import (
	"bytes"
	"fmt"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf16"
	"unsafe"
)

var convert = syscall.NewLazyDLL("kernel32.dll").NewProc("WideCharToMultiByte")

// Lines applies a conservative Font A width for common 58/80 mm printers.
func Lines(lines []string, paperWidth int) []string {
	width := 48
	if paperWidth == 58 {
		width = 32
	}
	var result []string
	for _, line := range lines {
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return ' '
			}
			return r
		}, line)
		chars := []rune(line)
		for len(chars) > width {
			end := width
			for i := width; i > 0; i-- {
				if chars[i] == ' ' {
					end = i
					break
				}
			}
			result = append(result, string(chars[:end]))
			chars = chars[end:]
			for len(chars) > 0 && chars[0] == ' ' {
				chars = chars[1:]
			}
		}
		result = append(result, string(chars))
	}
	return result
}

// Encode selects PC850 (ESC t 2), feeds paper, then requests a partial cut.
func Encode(lines []string) ([]byte, error) {
	text := utf16.Encode([]rune(strings.Join(lines, "\n") + "\n"))
	data := make([]byte, len(text))
	n, _, err := convert.Call(850, 0x400, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0, 0)
	if n == 0 {
		return nil, fmt.Errorf("convertir ticket a CP850: %w", err)
	}
	var result bytes.Buffer
	result.WriteString("\x1b@\x1bt\x02")
	result.Write(data[:n])
	result.WriteString("\x1bd\x03\x1dVB\x00")
	return result.Bytes(), nil
}
