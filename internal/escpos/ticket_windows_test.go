package escpos

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestEncodeWithLogo(t *testing.T) {
	logo := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	logo.Set(0, 0, color.Black)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, logo); err != nil {
		t.Fatal(err)
	}
	lines := []string{"Food Truck", "PRECUENTA"}
	plain, err := Encode(lines, 80, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	withLogo, err := EncodeReceipt(lines, 80, 2, false, encoded.Bytes(), "")
	if err != nil {
		t.Fatal(err)
	}
	const init = "\x1b@\x1bt\x02"
	const raster = "\x1ba\x01\x1dv0\x00\x01\x00\x01\x00\x80\n\x1ba\x00"
	want := append([]byte(init+raster), plain[len(init):]...)
	if !bytes.Equal(withLogo, want) {
		t.Fatalf("ticket con logo = %q, esperado %q", withLogo, want)
	}
	fallback, err := EncodeReceipt(lines, 80, 2, false, []byte("imagen inválida"), "")
	if err != nil || !bytes.Equal(fallback, plain) {
		t.Fatalf("la imagen inválida cambió el ticket: %q, %v", fallback, err)
	}
}

func TestReceiptQRBeforeCut(t *testing.T) {
	const url = "https://example.com/ticket#00000000-0000-0000-0000-000000000000.signature-for-test"
	const qrPrefix = "Verifica tu ticket\n\x1dv0\x00"
	const suffix = "\n\x1ba\x00\x1bd\x03\x1dVB\x00"
	for _, tc := range []struct {
		name       string
		paperWidth int
		wantSize   int
	}{
		{"58 mm", 58, 320},
		{"80 mm", 80, 384},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := EncodeReceipt([]string{"PAGADO"}, tc.paperWidth, 0, false, nil, url)
			if err != nil {
				t.Fatal(err)
			}
			_, raster, found := bytes.Cut(data, []byte(qrPrefix))
			if !found || len(raster) < 4 || !bytes.HasSuffix(raster, []byte(suffix)) {
				t.Fatal("el QR debe quedar después del texto y antes del corte")
			}
			stride := int(binary.LittleEndian.Uint16(raster[:2]))
			height := int(binary.LittleEndian.Uint16(raster[2:4]))
			if stride*8 != tc.wantSize || height != tc.wantSize {
				t.Fatalf("QR = %d × %d puntos, esperado %d × %d", stride*8, height, tc.wantSize, tc.wantSize)
			}
			if got := len(raster) - 4 - len(suffix); got != stride*height {
				t.Fatalf("datos del raster = %d bytes, esperados %d", got, stride*height)
			}
		})
	}
}

func TestEncodeStyledTicket(t *testing.T) {
	data, err := Encode([]string{
		"Restaurante Nombre Muy Largo", "Av. Principal", "PRECUENTA", "Pedido #1",
	}, 58, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("\x1b@\x1bt\x02\x1ba\x01\x1bE\x01\x1d!\x11" +
		"Restaurante\nNombre Muy Largo\n\x1d!\x00\x1bE\x00Av. Principal\n" +
		"\x1bE\x01PRECUENTA\n\x1bE\x00\x1ba\x00Pedido #1\n\x1bd\x03\x1dVB\x00")
	if !bytes.Equal(data, want) {
		t.Fatalf("ticket = %q, esperado %q", data, want)
	}
}

func TestEncodeLegacyTicket(t *testing.T) {
	data, err := Encode([]string{"Prueba"}, 80, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte("\x1b@\x1bt\x02Prueba\n\x1bd\x03\x1dVB\x00"); !bytes.Equal(data, want) {
		t.Fatalf("ticket = %q, esperado %q", data, want)
	}
}

func TestEncodePaymentFooter(t *testing.T) {
	data, err := Encode([]string{
		"Estado: PAGADO", "--------------------------------", "Gracias por su visita",
	}, 80, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("\x1b@\x1bt\x02Estado: PAGADO\n--------------------------------\n" +
		"\x1ba\x01\x1bE\x01\x1d!\x01Gracias por su visita\n" +
		"\x1d!\x00\x1bE\x00\x1ba\x00\x1bd\x03\x1dVB\x00")
	if !bytes.Equal(data, want) {
		t.Fatalf("ticket = %q, esperado %q", data, want)
	}
}
