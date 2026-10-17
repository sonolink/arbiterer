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
	"os"
	"path/filepath"
	"testing"
)

// writeSecretFile writes contents to a file in a temporary directory and
// returns its path.
func writeSecretFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing secret file: %v", err)
	}

	return path
}

// TestSecretReadsTheEnvironment checks the plain case, where no file is named.
//
// These tests cannot run in parallel because t.Setenv forbids it.
func TestSecretReadsTheEnvironment(t *testing.T) {
	t.Setenv("ARBITERER_TEST_SECRET", "from-the-environment")

	got, err := secret("ARBITERER_TEST_SECRET")
	if err != nil {
		t.Fatalf("secret: %v", err)
	}

	if got != "from-the-environment" {
		t.Errorf("secret = %q, want %q", got, "from-the-environment")
	}
}

// TestSecretRejectsBothSources checks that a half-migrated deployment fails at
// startup. Choosing one silently would let a rotation of the unused source look
// like it took effect.
func TestSecretRejectsBothSources(t *testing.T) {
	t.Setenv("ARBITERER_TEST_SECRET", "from-the-environment")
	t.Setenv("ARBITERER_TEST_SECRET_FILE", writeSecretFile(t, "from-the-file"))

	if _, err := secret("ARBITERER_TEST_SECRET"); err == nil {
		t.Error("secret accepted both sources, want an error")
	}
}

// TestSecretReadsTheFile checks the Docker secrets path.
func TestSecretReadsTheFile(t *testing.T) {
	t.Setenv("ARBITERER_TEST_SECRET_FILE", writeSecretFile(t, "from-the-file"))

	got, err := secret("ARBITERER_TEST_SECRET")
	if err != nil {
		t.Fatalf("secret: %v", err)
	}

	if got != "from-the-file" {
		t.Errorf("secret = %q, want %q", got, "from-the-file")
	}
}

// TestSecretTrimsTheTrailingNewline checks the line that makes Docker secrets
// usable at all. A file written with echo ends in a newline, and an untrimmed
// key decodes to the wrong length for a reason its error never mentions.
func TestSecretTrimsTheTrailingNewline(t *testing.T) {
	for name, contents := range map[string]string{
		"unix":    "a-secret\n",
		"windows": "a-secret\r\n",
		"several": "a-secret\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ARBITERER_TEST_SECRET_FILE", writeSecretFile(t, contents))

			got, err := secret("ARBITERER_TEST_SECRET")
			if err != nil {
				t.Fatalf("secret: %v", err)
			}

			if got != "a-secret" {
				t.Errorf("secret = %q, want %q", got, "a-secret")
			}
		})
	}
}

// TestSecretKeepsInnerWhitespace checks that only the trailing newline goes. A
// secret is otherwise taken exactly as written.
func TestSecretKeepsInnerWhitespace(t *testing.T) {
	t.Setenv("ARBITERER_TEST_SECRET_FILE", writeSecretFile(t, "-----BEGIN KEY-----\nline\n-----END KEY-----\n"))

	got, err := secret("ARBITERER_TEST_SECRET")
	if err != nil {
		t.Fatalf("secret: %v", err)
	}

	want := "-----BEGIN KEY-----\nline\n-----END KEY-----"
	if got != want {
		t.Errorf("secret = %q, want %q", got, want)
	}
}

// TestSecretRejectsMissingValues checks that a misconfigured deployment fails
// at startup rather than running with an empty secret.
func TestSecretRejectsMissingValues(t *testing.T) {
	t.Run("nothing set", func(t *testing.T) {
		t.Setenv("ARBITERER_TEST_SECRET", "")

		if _, err := secret("ARBITERER_TEST_SECRET"); err == nil {
			t.Error("secret accepted an unset variable, want an error")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		t.Setenv("ARBITERER_TEST_SECRET_FILE", filepath.Join(t.TempDir(), "absent"))

		if _, err := secret("ARBITERER_TEST_SECRET"); err == nil {
			t.Error("secret accepted a missing file, want an error")
		}
	})

	t.Run("file is empty", func(t *testing.T) {
		t.Setenv("ARBITERER_TEST_SECRET_FILE", writeSecretFile(t, "\n"))

		if _, err := secret("ARBITERER_TEST_SECRET"); err == nil {
			t.Error("secret accepted an empty file, want an error")
		}
	})
}
