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
	return wrapLines(lines, columns(paperWidth))
}

func columns(paperWidth int) int {
	if paperWidth == 58 {
		return 32
	}
	return 48
}

func wrapLines(lines []string, width int) []string {
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
func encodeText(lines []string) ([]byte, error) {
	text := utf16.Encode([]rune(strings.Join(lines, "\n") + "\n"))
	data := make([]byte, len(text))
	n, _, err := convert.Call(850, 0x400, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0, 0)
	if n == 0 {
		return nil, fmt.Errorf("convertir ticket a CP850: %w", err)
	}
	return data[:n], nil
}

// Encode wraps plain and styled tickets. The optional header occupies the first
// lines of the payload, with the restaurant first and the title last.
func Encode(lines []string, paperWidth, headerLines int) ([]byte, error) {
	var result bytes.Buffer
	result.WriteString("\x1b@\x1bt\x02")
	appendText := func(text []string) error {
		if len(text) == 0 {
			return nil
		}
		data, err := encodeText(text)
		if err != nil {
			return err
		}
		result.Write(data)
		return nil
	}
	if headerLines >= 2 && headerLines <= len(lines) {
		result.WriteString("\x1ba\x01\x1bE\x01\x1d!\x11")
		if err := appendText(wrapLines(lines[:1], columns(paperWidth)/2)); err != nil {
			return nil, err
		}
		result.WriteString("\x1d!\x00\x1bE\x00")
		if err := appendText(Lines(lines[1:headerLines-1], paperWidth)); err != nil {
			return nil, err
		}
		result.WriteString("\x1bE\x01")
		if err := appendText(Lines(lines[headerLines-1:headerLines], paperWidth)); err != nil {
			return nil, err
		}
		result.WriteString("\x1bE\x00\x1ba\x00")
		lines = lines[headerLines:]
	}
	if err := appendText(Lines(lines, paperWidth)); err != nil {
		return nil, err
	}
	result.WriteString("\x1bd\x03\x1dVB\x00")
	return result.Bytes(), nil
}
