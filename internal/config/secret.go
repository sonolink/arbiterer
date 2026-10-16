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
	"encoding"
	"fmt"
	"os"
	"strings"
)

// secret returns the value of the secret named name. When name_FILE is set it
// names a file to read the value from (Docker secrets); otherwise name itself
// is read from the environment.
func secret(name string) (string, error) {
	value := os.Getenv(name)

	if path := os.Getenv(name + "_FILE"); path != "" {
		contents, err := os.ReadFile(path) //nolint:gosec // path comes from the deployment.
		if err != nil {
			return "", fmt.Errorf("config: reading %s from %q: %w", name, path, err)
		}

		value = strings.TrimRight(string(contents), "\r\n")
	}

	if value == "" {
		return "", fmt.Errorf("config: %s is not set", name)
	}

	return value, nil
}

// decodeSecret decodes the secret named name into dst.
func decodeSecret(name string, dst encoding.TextUnmarshaler) error {
	value, err := secret(name)
	if err != nil {
		return err
	}

	if err := dst.UnmarshalText([]byte(value)); err != nil {
		return fmt.Errorf("config: parsing %s: %w", name, err)
	}

	return nil
}

// Secret is a string config value resolved from the environment or a Docker
// secret file.
type Secret string

// UnmarshalText stores the text as the secret value.
func (s *Secret) UnmarshalText(text []byte) error {
	*s = Secret(text)

	return nil
}
