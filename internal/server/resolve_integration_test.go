//go:build integration

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
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sonolink/arbiterer/internal/github"
)

// TestResolveRequiresAValidToken checks the gate the endpoint sits behind. The
// claims decide which repository a request may act on, so a token the app
// cannot verify has to be refused before anything is read or written.
func TestResolveRequiresAValidToken(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	// An issuer the app does not trust, signing with a key it never published.
	other := newOIDCIssuer(t)

	tests := map[string]string{
		"no token at all": "",
		"not a token":     "not-a-token",
		"signed by another issuer": other.token(t, github.Claims{
			Repository:   testRepository,
			RepositoryID: nextID(),
		}),
	}

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status, _ := h.post(t, "/v1/discord/resolve", token, `{"github_user_id":"1","issue_number":1}`)
			if status != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", status, http.StatusUnauthorized)
			}
		})
	}
}

// TestResolveUnlinkedAuthor walks the gate for a contributor the app has never
// seen, it should answer unlinked with somewhere to go, post the setup comment
// through GitHub, and remember the comment so the next event edits that one
// instead of posting another.
func TestResolveUnlinkedAuthor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	repositoryID := nextID()
	issueNumber := nextID()

	token := h.issuer.token(t, github.Claims{
		Repository:        testRepository,
		RepositoryID:      repositoryID,
		RepositoryOwnerID: nextID(),
		RunID:             nextID(),
	})

	status, body := h.post(t, "/v1/discord/resolve", token, fmt.Sprintf(
		`{"github_user_id":"555","issue_number":%d}`, issueNumber,
	))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", status, http.StatusOK, body)
	}

	var resp resolveResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}

	if resp.Status != statusUnlinked {
		t.Errorf("status = %q, want %q", resp.Status, statusUnlinked)
	}

	if resp.LinkURL == "" {
		t.Error("link_url is empty, so the author is told to link with nowhere to do it")
	}

	comments := fmt.Sprintf("/repos/%s/issues/%d/comments", testRepository, issueNumber)
	if !h.github.called(http.MethodPost, comments) {
		t.Errorf("no comment was posted to %s", comments)
	}

	stored, err := testStore.SetupCommentByPullRequest(t.Context(), repositoryID, issueNumber)
	if err != nil {
		t.Fatalf("SetupCommentByPullRequest: %v", err)
	}

	if stored.CommentID != commentID {
		t.Errorf("stored comment id = %d, want %d", stored.CommentID, commentID)
	}

	if stored.Status != string(commentUnlinked) {
		t.Errorf("stored status = %q, want %q", stored.Status, commentUnlinked)
	}
}
