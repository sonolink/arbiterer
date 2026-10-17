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

package server

import (
	"context"
	"errors"
	"time"
)

// permanentError wraps a failure that retrying cannot fix.
type permanentError struct {
	err error
}

func (pe permanentError) Error() string {
	return pe.err.Error()
}

func (pe permanentError) Unwrap() error {
	return pe.err
}

// permanent marks err as one that will fail the same way however often it is
// tried, such as an update of a row that no longer exists.
func permanent(err error) error {
	return permanentError{err: err}
}

// retry calls fn until it succeeds, it returns an error marked by permanent, or
// attempts are spent. Each wait doubles, starting at backoff, and ends early
// when ctx does. A permanent error is unwrapped before it is returned, so
// callers match it as fn produced it.
func retry(ctx context.Context, attempts int, backoff time.Duration, fn func() error) error {
	var err error

	for attempt := 1; attempt <= attempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}

		var pe permanentError
		if errors.As(err, &pe) {
			return pe.err
		}

		if attempt == attempts {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return err
}
