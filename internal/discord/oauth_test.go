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

package discord

import (
	"slices"
	"testing"
	"time"
)

// bearer is a response as Discord sends it, for cases that vary one field.
var bearer = tokenResponse{
	AccessToken:  "an-access-token",
	TokenType:    "Bearer",
	ExpiresIn:    604800,
	RefreshToken: "a-refresh-token",
	Scope:        "identify guilds.members.read",
}

// TestTokenResponseTokenCarriesTheFields checks the happy path, including the
// expiry, which Discord sends as a lifetime in seconds and we store as an
// instant.
func TestTokenResponseTokenCarriesTheFields(t *testing.T) {
	t.Parallel()

	// token is read against the clock, so bound it by both sides of the call
	// rather than comparing to a single Now.
	before := time.Now()

	token, err := bearer.token()
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	after := time.Now()

	if token.AccessToken != bearer.AccessToken {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, bearer.AccessToken)
	}

	if token.RefreshToken != bearer.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", token.RefreshToken, bearer.RefreshToken)
	}

	lifetime := time.Duration(bearer.ExpiresIn) * time.Second
	if token.ExpiresAt.Before(before.Add(lifetime)) || token.ExpiresAt.After(after.Add(lifetime)) {
		t.Errorf("ExpiresAt = %v, want within %v of now", token.ExpiresAt, lifetime)
	}
}

// TestTokenResponseTokenRejectsOtherTypes checks that only a bearer token is
// accepted, whatever case it arrives in.
func TestTokenResponseTokenRejectsOtherTypes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		tokenType string
		wantErr   bool
	}{
		"bearer as documented": {tokenType: "Bearer"},
		"bearer in lower case": {tokenType: "bearer"},
		"bearer in upper case": {tokenType: "BEARER"},
		"another scheme":       {tokenType: "Basic", wantErr: true},
		"no type at all":       {wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tr := bearer
			tr.TokenType = tc.tokenType

			_, err := tr.token()
			if tc.wantErr && err == nil {
				t.Errorf("token accepted token_type %q, want an error", tc.tokenType)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("token rejected token_type %q: %v", tc.tokenType, err)
			}
		})
	}
}

// TestTokenResponseTokenRejectsAnEmptyAccessToken checks that a response
// missing the one field we are asking for is caught here, rather than being
// stored and failing at the next call that uses it.
func TestTokenResponseTokenRejectsAnEmptyAccessToken(t *testing.T) {
	t.Parallel()

	tr := bearer
	tr.AccessToken = ""

	if _, err := tr.token(); err == nil {
		t.Error("token accepted an empty access_token, want an error")
	}
}

// TestTokenResponseTokenSplitsTheScopes checks that the granted scopes are split
// out of the single space separated string Discord sends. They can differ from
// what was asked for, so what comes back is what gets stored.
func TestTokenResponseTokenSplitsTheScopes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		scope string
		want  []string
	}{
		"the scopes we ask for": {
			scope: bearer.Scope,
			want:  []string{"identify", "guilds.members.read"},
		},
		"a single scope":   {scope: "email", want: []string{"email"}},
		"no scopes at all": {scope: "", want: nil},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tr := bearer
			tr.Scope = tc.scope

			token, err := tr.token()
			if err != nil {
				t.Fatalf("token: %v", err)
			}

			if !slices.Equal(token.Scopes, tc.want) {
				t.Errorf("Scopes = %q, want %q", token.Scopes, tc.want)
			}
		})
	}
}
