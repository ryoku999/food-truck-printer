package escpos

import (
	"bytes"
	"testing"
)

func TestEncodeStyledTicket(t *testing.T) {
	data, err := Encode([]string{
		"Restaurante Nombre Muy Largo", "Av. Principal", "PRECUENTA", "Pedido #1",
	}, 58, 3)
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
	data, err := Encode([]string{"Prueba"}, 80, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte("\x1b@\x1bt\x02Prueba\n\x1bd\x03\x1dVB\x00"); !bytes.Equal(data, want) {
		t.Fatalf("ticket = %q, esperado %q", data, want)
	}
}
