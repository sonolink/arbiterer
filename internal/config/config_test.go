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

package config

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

// TestKeyUnmarshalText checks that only a 32 byte key is accepted (AES takes 16, 24 or 32).
func TestKeyUnmarshalText(t *testing.T) {
	t.Parallel()

	// encode returns size random bytes as a key would be written in the
	// environment.
	encode := func(t *testing.T, size int) string {
		t.Helper()

		key := make([]byte, size)
		if _, err := rand.Read(key); err != nil {
			t.Fatalf("reading random bytes: %v", err)
		}

		return base64.StdEncoding.EncodeToString(key)
	}

	tests := map[string]struct {
		text    string
		wantErr bool
	}{
		"a 32 byte key is accepted": {text: encode(t, keySize)},
		"a 16 byte key is rejected": {text: encode(t, 16), wantErr: true},
		"a 24 byte key is rejected": {text: encode(t, 24), wantErr: true},
		"a 33 byte key is rejected": {text: encode(t, 33), wantErr: true},
		"a key that is not base64 is rejected": {
			text:    "not base64",
			wantErr: true,
		},
		"an empty key is rejected": {wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var key Key

			err := key.UnmarshalText([]byte(tc.text))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("UnmarshalText accepted %q, want an error", tc.text)
				}

				return
			}

			if err != nil {
				t.Fatalf("UnmarshalText: %v", err)
			}

			want, err := base64.StdEncoding.DecodeString(tc.text)
			if err != nil {
				t.Fatalf("decoding the key: %v", err)
			}

			if !bytes.Equal(key, want) {
				t.Errorf("key = %x, want %x", key, want)
			}
		})
	}
}

// TestRSAPrivateKeyUnmarshalText checks that a key is read whether it arrives as
// PEM or as base64 wrapping that PEM, which is how it is kept on one line in an
// environment variable.
func TestRSAPrivateKeyUnmarshalText(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}

	pemKey := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))

	tests := map[string]struct {
		text    string
		wantErr bool
	}{
		"pem is parsed as it stands": {text: pemKey},
		"base64 wrapping pem is decoded first": {
			text: base64.StdEncoding.EncodeToString([]byte(pemKey)),
		},
		"surrounding whitespace is ignored": {text: "\n  " + pemKey + "  \n"},
		"base64 that is not pem is rejected": {
			text:    base64.StdEncoding.EncodeToString([]byte("not a key")),
			wantErr: true,
		},
		"text that is neither is rejected": {text: "not a key", wantErr: true},
		"an empty key is rejected":         {wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got RSAPrivateKey

			err := got.UnmarshalText([]byte(tc.text))
			if tc.wantErr {
				if err == nil {
					t.Fatal("UnmarshalText accepted the value, want an error")
				}

				return
			}

			if err != nil {
				t.Fatalf("UnmarshalText: %v", err)
			}

			if !key.Equal((*rsa.PrivateKey)(&got)) {
				t.Error("UnmarshalText produced a different key")
			}
		})
	}
}
