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

package github

import (
	"errors"
	"fmt"
)

// APIError is an error returned by a GitHub REST endpoint.
type APIError struct {
	Status  int
	Message string
	Body    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github: http %d: %s (body: %.200q)", e.Status, e.Message, e.Body)
}

// ErrAppNotInstalled reports that the GitHub App is not installed on the
// repository it was asked to act on.
var ErrAppNotInstalled = errors.New("github: app not installed on repository")
