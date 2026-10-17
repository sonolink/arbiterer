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
	"net/url"
	"testing"
	"time"

	"github.com/sonolink/arbiterer/internal/config"
	"github.com/sonolink/arbiterer/internal/secrets"
)

// newLinkServer builds a Server with only what the linking primitives read. A
// negative lifetime seals a value that is already expired.
func newLinkServer(t *testing.T, lifetime time.Duration) *Server {
	t.Helper()

	sealer, err := secrets.NewSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	return &Server{
		cfg: config.Server{
			PublicURL:          "https://arbiterer.dev",
			LinkTokenLifetime:  lifetime,
			LinkCookieLifetime: lifetime,
		},
		browserSealer: sealer,
	}
}

// Fixture values shared by the tests in this package.
const (
	testNonce      = "a-nonce"
	testRepository = "sonolink/arbiterer"
)

func testLinkToken() linkToken {
	return linkToken{
		GitHubUserID: "555",
		RepositoryID: 42,
		Repository:   testRepository,
		IssueNumber:  7,
		Nonce:        testNonce,
	}
}

// TestLinkTokenRoundTrip checks that a sealed token carries its payload back
// intact, so the Discord step acts on what resolve issued.
func TestLinkTokenRoundTrip(t *testing.T) {
	t.Parallel()

	s := newLinkServer(t, time.Minute)
	want := testLinkToken()

	encoded, err := s.sealLinkToken(want, linkTokenAAD)
	if err != nil {
		t.Fatalf("sealLinkToken: %v", err)
	}

	got, err := s.openLinkToken(encoded, linkTokenAAD)
	if err != nil {
		t.Fatalf("openLinkToken: %v", err)
	}

	// sealLinkToken sets Expiry itself, so it is not part of the payload.
	got.Expiry = time.Time{}

	if got != want {
		t.Errorf("openLinkToken = %+v, want %+v", got, want)
	}
}

// TestOpenLinkTokenRejectsExpired checks the expiry gate on a bearer token that
// sits in a public pull request comment.
func TestOpenLinkTokenRejectsExpired(t *testing.T) {
	t.Parallel()

	s := newLinkServer(t, -time.Minute)

	encoded, err := s.sealLinkToken(testLinkToken(), linkTokenAAD)
	if err != nil {
		t.Fatalf("sealLinkToken: %v", err)
	}

	if _, err := s.openLinkToken(encoded, linkTokenAAD); err == nil {
		t.Error("openLinkToken accepted an expired token, want an error")
	}
}

// TestOpenLinkTokenRejectsOtherContexts checks the domain separation between
// the payloads the browser sealer protects. One key seals them all, so only the
// additional data stops a value from one context being replayed as another.
func TestOpenLinkTokenRejectsOtherContexts(t *testing.T) {
	t.Parallel()

	s := newLinkServer(t, time.Minute)

	encoded, err := s.sealLinkToken(testLinkToken(), linkTokenAAD)
	if err != nil {
		t.Fatalf("sealLinkToken: %v", err)
	}

	others := map[string]string{
		"link state":        linkStateAAD,
		"link cookie":       linkCookieAAD,
		"link state cookie": linkStateCookieAAD,
	}

	for name, aad := range others {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := s.openLinkToken(encoded, aad); err == nil {
				t.Errorf("openLinkToken accepted a link token opened as %s, want an error", name)
			}
		})
	}
}

// TestLinkCookieRoundTrip checks the handoff from the GitHub step to the
// Discord step survives sealing.
func TestLinkCookieRoundTrip(t *testing.T) {
	t.Parallel()

	s := newLinkServer(t, time.Minute)
	want := linkCookie{
		Nonce:        testNonce,
		RepositoryID: 42,
		GitHubUserID: "555",
		Repository:   testRepository,
		IssueNumber:  7,
	}

	encoded, err := s.sealLinkCookie(want)
	if err != nil {
		t.Fatalf("sealLinkCookie: %v", err)
	}

	got, err := s.openLinkCookie(encoded)
	if err != nil {
		t.Fatalf("openLinkCookie: %v", err)
	}

	got.Expiry = time.Time{}

	if got != want {
		t.Errorf("openLinkCookie = %+v, want %+v", got, want)
	}
}

// TestOpenLinkCookieRejectsExpired checks that a stale handoff cannot resume a
// linking attempt.
func TestOpenLinkCookieRejectsExpired(t *testing.T) {
	t.Parallel()

	s := newLinkServer(t, -time.Minute)

	encoded, err := s.sealLinkCookie(linkCookie{Nonce: testNonce})
	if err != nil {
		t.Fatalf("sealLinkCookie: %v", err)
	}

	if _, err := s.openLinkCookie(encoded); err == nil {
		t.Error("openLinkCookie accepted an expired cookie, want an error")
	}
}

// TestLinkURL checks the link handed to a contributor, including a PublicURL
// configured with a trailing slash.
func TestLinkURL(t *testing.T) {
	t.Parallel()

	for name, publicURL := range map[string]string{
		"without trailing slash": "https://arbiterer.dev",
		"with trailing slash":    "https://arbiterer.dev/",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := newLinkServer(t, time.Minute)
			s.cfg.PublicURL = publicURL

			raw, err := s.linkURL(testLinkToken())
			if err != nil {
				t.Fatalf("linkURL: %v", err)
			}

			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parsing %q: %v", raw, err)
			}

			if u.Host != "arbiterer.dev" {
				t.Errorf("host = %q, want %q", u.Host, "arbiterer.dev")
			}

			if u.Path != linkPath {
				t.Errorf("path = %q, want %q", u.Path, linkPath)
			}

			if u.Query().Get("token") == "" {
				t.Error("token query parameter is empty")
			}
		})
	}
}
