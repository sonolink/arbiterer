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
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestCommentActionsFor covers which links each comment offers.
func TestCommentActionsFor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status      commentStatus
		wantRecheck bool
		wantAccount bool
	}{
		"a rules failure offers a recheck and account links": {
			status:      commentRulesFailed,
			wantRecheck: true,
			wantAccount: true,
		},
		"a running recheck offers a retry and account links": {
			status:      commentRechecking,
			wantRecheck: true,
			wantAccount: true,
		},
		"a pass offers only account links": {
			status:      commentPassed,
			wantAccount: true,
		},
		"a link confirmation offers only account links": {
			status:      commentLinked,
			wantAccount: true,
		},
		"an unlinked author has nothing to unlink": {
			status: commentUnlinked,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := newLinkServer(t, time.Minute)
			s.cfg.ActionTokenLifetime = time.Hour

			actions, err := s.commentActionsFor(tc.status, 42, testRepository, 7, 555)
			if err != nil {
				t.Fatalf("commentActionsFor: %v", err)
			}

			for _, link := range []struct {
				url    string
				action commentAction
				want   bool
			}{
				{actions.Recheck, actionRecheck, tc.wantRecheck},
				{actions.Unlink, actionUnlink, tc.wantAccount},
				{actions.Delete, actionDelete, tc.wantAccount},
			} {
				if (link.url != "") != link.want {
					t.Fatalf("%s link present = %t, want %t", link.action, link.url != "", link.want)
				}

				if !link.want {
					continue
				}

				u, err := url.Parse(link.url)
				if err != nil {
					t.Fatalf("parsing %s link: %v", link.action, err)
				}

				if u.Path != actionPath {
					t.Errorf("%s link path = %q, want %q", link.action, u.Path, actionPath)
				}

				lt, err := s.openLinkToken(u.Query().Get("token"), actionTokenAAD)
				if err != nil {
					t.Fatalf("opening %s token: %v", link.action, err)
				}

				if lt.Action != link.action || lt.GitHubUserID != "555" || lt.IssueNumber != 7 {
					t.Errorf("%s token = %+v", link.action, lt)
				}

				// An action token must not open as a link token.
				if _, err := s.openLinkToken(u.Query().Get("token"), linkTokenAAD); err == nil {
					t.Errorf("%s token opened as a link token", link.action)
				}
			}
		})
	}
}

// TestFormatCommentActions covers the action line under the comment.
func TestFormatCommentActions(t *testing.T) {
	t.Parallel()

	if got := formatCommentActions(commentActions{}); got != "" {
		t.Errorf("no actions rendered %q, want nothing", got)
	}

	got := formatCommentActions(commentActions{Unlink: "https://u", Delete: "https://d"})
	if strings.Contains(got, "Recheck") {
		t.Errorf("rendered a recheck link that was not set: %q", got)
	}

	for _, want := range []string{"[Unlink](https://u)", "(https://d)"} {
		if !strings.Contains(got, want) {
			t.Errorf("actions %q missing %q", got, want)
		}
	}
}

// TestWriteActionPage covers that the page carries the token to post back and
// escapes what it shows.
func TestWriteActionPage(t *testing.T) {
	t.Parallel()

	s := newLinkServer(t, time.Minute)

	rec := httptest.NewRecorder()
	s.writeActionPage(rec, http.StatusOK, actionPage{
		Action:  string(actionUnlink),
		Repo:    "<script>",
		PRURL:   "https://github.com/" + testRepository + "/pull/7",
		PostURL: actionPath + "/confirm",
		Token:   "sealed",
	})

	body := rec.Body.String()
	for _, want := range []string{`name="token" value="sealed"`, `action="/v1/link/action/confirm"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}

	if strings.Contains(body, "for <script>") {
		t.Error("page does not escape the repository name")
	}

	rec = httptest.NewRecorder()
	s.writeActionPage(rec, http.StatusBadRequest, actionPage{Error: linkDetailIdentityMismatch})

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), linkDetailIdentityMismatch) {
		t.Errorf("error page = %d %q", rec.Code, rec.Body.String())
	}

	if strings.Contains(rec.Body.String(), "<form") {
		t.Error("error page asks for confirmation")
	}
}

// TestFormatCommentActionsOrder covers that the links keep their order, so
// Recheck stays first and Delete my data stays last.
func TestFormatCommentActionsOrder(t *testing.T) {
	t.Parallel()

	got := formatCommentActions(commentActions{
		Recheck: "https://r",
		Unlink:  "https://u",
		Delete:  "https://d",
	})

	want := "\n\n[Recheck](https://r) · [Unlink](https://u) · [Delete my data](https://d)"
	if got != want {
		t.Errorf("formatCommentActions = %q, want %q", got, want)
	}
}

// TestActionURLUsesTheActionLifetime checks that action links are sealed with
// their own lifetime. They sit in the comment for as long as the pull request
// is open, so they must not expire with the short lived link token.
func TestActionURLUsesTheActionLifetime(t *testing.T) {
	t.Parallel()

	s := newLinkServer(t, -time.Minute)
	s.cfg.ActionTokenLifetime = time.Hour

	link, err := s.actionURL(testLinkToken(), actionUnlink)
	if err != nil {
		t.Fatalf("actionURL: %v", err)
	}

	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parsing link: %v", err)
	}

	if _, err := s.openLinkToken(u.Query().Get(tokenParam), actionTokenAAD); err != nil {
		t.Errorf("openLinkToken rejected an action token within its lifetime: %v", err)
	}
}

// newActionServer builds a Server that can answer the action endpoints up to
// the point where they would call GitHub or the database.
func newActionServer(t *testing.T) *Server {
	t.Helper()

	s := newLinkServer(t, time.Minute)
	s.cfg.ActionTokenLifetime = time.Hour
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	return s
}

// sealAction seals lt for action under aad.
func sealAction(t *testing.T, s *Server, action commentAction, aad string) string {
	t.Helper()

	lt := testLinkToken()
	lt.Action = action

	token, err := s.sealLinkToken(lt, aad)
	if err != nil {
		t.Fatalf("sealLinkToken: %v", err)
	}

	return token
}

// TestHandleCommentActionRejects covers the tokens the action link refuses
// before it sends the author to GitHub.
func TestHandleCommentActionRejects(t *testing.T) {
	t.Parallel()

	s := newActionServer(t)

	tests := map[string]string{
		"no token":          "",
		"not a token":       "not-a-token",
		"a link token":      sealAction(t, s, actionUnlink, linkTokenAAD),
		"a confirm token":   sealAction(t, s, actionUnlink, actionConfirmAAD),
		"no action":         sealAction(t, s, "", actionTokenAAD),
		"an unknown action": sealAction(t, s, "merge", actionTokenAAD),
	}

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			target := actionPath + "?" + url.Values{tokenParam: {token}}.Encode()
			rec := httptest.NewRecorder()
			s.handleCommentAction(rec, httptest.NewRequest(http.MethodGet, target, nil))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}

			if !strings.Contains(rec.Body.String(), actionDetailExpired) {
				t.Errorf("page does not say the link expired: %q", rec.Body.String())
			}

			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("redirected to %q, want no redirect", loc)
			}
		})
	}
}

// TestHandleCommentActionConfirmRejects covers the tokens the confirmation
// refuses before it touches the database. An action link token must not be
// accepted here, or the confirmation page could be skipped.
func TestHandleCommentActionConfirmRejects(t *testing.T) {
	t.Parallel()

	s := newActionServer(t)

	tests := map[string]string{
		"no token":                   "",
		"an action link token":       sealAction(t, s, actionDelete, actionTokenAAD),
		"a recheck does not confirm": sealAction(t, s, actionRecheck, actionConfirmAAD),
	}

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			form := url.Values{tokenParam: {token}}.Encode()
			req := httptest.NewRequest(http.MethodPost, actionPath+"/confirm", strings.NewReader(form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rec := httptest.NewRecorder()
			s.handleCommentActionConfirm(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}

			if !strings.Contains(rec.Body.String(), actionDetailExpired) {
				t.Errorf("page does not say the link expired: %q", rec.Body.String())
			}
		})
	}
}

// TestPullRequestURL covers where the author is sent back to.
func TestPullRequestURL(t *testing.T) {
	t.Parallel()

	want := "https://github.com/" + testRepository + "/pull/7"
	if got := pullRequestURL(testLinkToken()); got != want {
		t.Errorf("pullRequestURL = %q, want %q", got, want)
	}
}
