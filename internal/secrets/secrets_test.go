package secrets_test

import (
	"bytes"
	"testing"

	"github.com/sonolink/arbiterer/internal/secrets"
)

func newSealer(t *testing.T) *secrets.Sealer {
	t.Helper()

	sealer, err := secrets.NewSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	return sealer
}

func TestSealOpenRoundTrip(t *testing.T) {
	t.Parallel()

	sealer := newSealer(t)
	plaintext := []byte("a discord access token")
	additionalData := []byte("123:access")

	sealed, err := sealer.Seal(plaintext, additionalData)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	opened, err := sealer.Open(sealed, additionalData)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if !bytes.Equal(opened, plaintext) {
		t.Errorf("Open = %q, want %q", opened, plaintext)
	}
}

func TestSealUsesAFreshNonce(t *testing.T) {
	t.Parallel()

	sealer := newSealer(t)
	plaintext := []byte("a discord access token")
	additionalData := []byte("123:access")

	first, err := sealer.Seal(plaintext, additionalData)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	second, err := sealer.Seal(plaintext, additionalData)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if bytes.Equal(first, second) {
		t.Error("Seal produced the same value twice, want a fresh nonce each time")
	}
}

func TestOpenAdditionalDataIsBound(t *testing.T) {
	t.Parallel()

	sealer := newSealer(t)
	plaintext := []byte("a discord access token")
	additionalData := []byte("123:access")

	sealed, err := sealer.Seal(plaintext, additionalData)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if _, err := sealer.Open(sealed, []byte("123:refresh")); err == nil {
		t.Error("Open with mismatched additionalData succeeded, want an error")
	}
}

func TestOpenRejectsShortValues(t *testing.T) {
	t.Parallel()

	sealer := newSealer(t)

	tests := map[string][]byte{
		"empty":              {},
		"shorter than nonce": {1, 2, 3},
		"nonce without tag":  bytes.Repeat([]byte{0}, 12),
	}

	for name, sealed := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := sealer.Open(sealed, nil); err == nil {
				t.Error("Open succeeded, want an error")
			}
		})
	}
}
