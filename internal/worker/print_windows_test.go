package worker

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestFallbackOnlyBeforeDelivery(t *testing.T) {
	failure := errors.New("destino no disponible")
	for _, test := range []struct {
		name, primary, fallback            string
		enabled, primarySafe, fallbackSafe bool
		primaryErr, fallbackErr            error
		wantSafe, wantError                bool
	}{
		{"USB unavailable", "SYSTEM_PRINTER", "NETWORK_TCP", true, true, false, failure, nil, false, false},
		{"TCP unavailable", "NETWORK_TCP", "SYSTEM_PRINTER", true, true, false, failure, nil, false, false},
		{"submission uncertain", "SYSTEM_PRINTER", "", true, false, false, failure, nil, false, true},
		{"primary accepted", "SYSTEM_PRINTER", "", true, false, false, nil, nil, false, false},
		{"fallback disabled", "NETWORK_TCP", "", false, true, false, failure, nil, true, true},
		{"both unavailable", "NETWORK_TCP", "SYSTEM_PRINTER", true, true, true, failure, failure, true, true},
		{"fallback uncertain", "NETWORK_TCP", "SYSTEM_PRINTER", true, true, false, failure, failure, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			safe, err := sendWithFallback(context.Background(), test.primary, test.enabled, func(transport string) (bool, error) {
				calls = append(calls, transport)
				if len(calls) == 1 {
					return test.primarySafe, test.primaryErr
				}
				return test.fallbackSafe, test.fallbackErr
			})
			want := []string{test.primary}
			if test.fallback != "" {
				want = append(want, test.fallback)
			}
			if !reflect.DeepEqual(calls, want) || safe != test.wantSafe || (err != nil) != test.wantError {
				t.Fatalf("calls=%v, safe=%t, error=%v", calls, safe, err)
			}
		})
	}
}
