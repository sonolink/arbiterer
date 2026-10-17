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
	"testing"
	"time"
)

// testBackoff keeps the waits short enough that exercising them doesn't cost much of ci time.
const testBackoff = time.Millisecond

var errTransient = errors.New("transient")

// TestRetryStopsOnSuccess checks that a call which works is not repeated.
func TestRetryStopsOnSuccess(t *testing.T) {
	t.Parallel()

	calls := 0

	err := retry(t.Context(), 3, testBackoff, func() error {
		calls++

		return nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// TestRetryRecovers checks that a transient failure is tried again.
func TestRetryRecovers(t *testing.T) {
	t.Parallel()

	calls := 0

	err := retry(t.Context(), 3, testBackoff, func() error {
		calls++
		if calls < 3 {
			return errTransient
		}

		return nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}

	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

// TestRetryExhausts checks that attempts are bounded and the last failure is
// what the caller sees.
func TestRetryExhausts(t *testing.T) {
	t.Parallel()

	calls := 0

	err := retry(t.Context(), 2, testBackoff, func() error {
		calls++

		return errTransient
	})

	if !errors.Is(err, errTransient) {
		t.Errorf("retry = %v, want %v", err, errTransient)
	}

	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

// TestRetryStopsOnPermanent checks that a permanent failure is not repeated and
// reaches the caller as the error the closure produced, so errors.Is still
// matches it upstream.
func TestRetryStopsOnPermanent(t *testing.T) {
	t.Parallel()

	calls := 0

	err := retry(t.Context(), 3, testBackoff, func() error {
		calls++

		return permanent(errTransient)
	})

	if !errors.Is(err, errTransient) {
		t.Errorf("retry = %v, want %v", err, errTransient)
	}

	var pe permanentError
	if errors.As(err, &pe) {
		t.Error("retry returned the permanent wrapper, want the error it wrapped")
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// TestRetryHonoursContext checks that a canceled request stops the waiting
// rather than spending the remaining attempts on a context that is already
// done.
func TestRetryHonoursContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	calls := 0

	err := retry(ctx, 3, time.Hour, func() error {
		calls++
		cancel()

		return errTransient
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("retry = %v, want %v", err, context.Canceled)
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}
