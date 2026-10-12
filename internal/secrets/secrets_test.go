/*
Copyright 2026-present SonoLink.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
		"nonce without tag":  make([]byte, 12),
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
