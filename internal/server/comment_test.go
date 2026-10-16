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
	"strings"
	"testing"

	"github.com/sonolink/arbiterer/internal/storage"
)

const testLinkURL = "https://arbiterer.test/v1/link?token=abc"

// TestSetupCommentNeedsWrite covers when a contributor is worth telling
// something. Writing when nothing changed costs an API call per event, and not
// writing when it did leaves them reading stale instructions.
func TestSetupCommentNeedsWrite(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		stored          *storage.SetupComment
		status          commentStatus
		linkURL         string
		commentOnLinked bool
		want            bool
	}{
		"first comment asks an unlinked author to act": {
			status:  commentUnlinked,
			linkURL: testLinkURL,
			want:    true,
		},
		"a linked author is left alone": {
			status: commentLinked,
			want:   false,
		},
		"unless the repository opted in to confirming": {
			status:          commentLinked,
			commentOnLinked: true,
			want:            true,
		},
		"an unchanged status with no link is not rewritten": {
			stored: &storage.SetupComment{Status: string(commentLinked)},
			status: commentLinked,
			want:   false,
		},
		"a changed status is rewritten": {
			stored: &storage.SetupComment{Status: string(commentUnlinked)},
			status: commentLinked,
			want:   true,
		},
		"a link is refreshed even when the status is unchanged": {
			stored:  &storage.SetupComment{Status: string(commentUnlinked)},
			status:  commentUnlinked,
			linkURL: testLinkURL,
			want:    true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := setupCommentNeedsWrite(tc.stored, tc.status, tc.linkURL, tc.commentOnLinked)
			if got != tc.want {
				t.Errorf("setupCommentNeedsWrite = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestSetupSteps covers which step a contributor is sent to. Linking the wrong
// one sends someone who already authorized GitHub back to the start.
func TestSetupSteps(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status     commentStatus
		wantDone   [2]bool
		wantLinkOn int // index of the step carrying linkURL, -1 for none
	}{
		"unlinked starts at GitHub": {
			status:     commentUnlinked,
			wantDone:   [2]bool{false, false},
			wantLinkOn: 0,
		},
		"revoked starts again at GitHub": {
			status:     commentRevoked,
			wantDone:   [2]bool{false, false},
			wantLinkOn: 0,
		},
		"a verified GitHub account resumes at Discord": {
			status:     commentGitHubVerified,
			wantDone:   [2]bool{true, false},
			wantLinkOn: 1,
		},
		"a linked author has nothing left to do": {
			status:     commentLinked,
			wantDone:   [2]bool{true, true},
			wantLinkOn: -1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			steps := setupSteps(tc.status, testLinkURL)
			if len(steps) != 2 {
				t.Fatalf("len(steps) = %d, want 2", len(steps))
			}

			for i, step := range steps {
				if step.done != tc.wantDone[i] {
					t.Errorf("steps[%d].done = %t, want %t", i, step.done, tc.wantDone[i])
				}

				if linked := strings.Contains(step.text, testLinkURL); linked != (i == tc.wantLinkOn) {
					t.Errorf("steps[%d] carries the link = %t, want %t", i, linked, i == tc.wantLinkOn)
				}
			}
		})
	}
}

// TestFormatCommentBody covers what the contributor reads. The assertions are
// structural so that rewording the copy does not fail the test.
func TestFormatCommentBody(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status    commentStatus
		linkURL   string
		wantSteps bool
		wantLink  bool
	}{
		"an unlinked author is given the steps": {
			status:    commentUnlinked,
			linkURL:   testLinkURL,
			wantSteps: true,
			wantLink:  true,
		},
		"a linked author is told so without a list of nothing to do": {
			status:    commentLinked,
			wantSteps: false,
			wantLink:  false,
		},
		"failing the rules is not a linking problem, so no steps": {
			status:    commentRulesFailed,
			linkURL:   testLinkURL,
			wantSteps: false,
			wantLink:  true,
		},
		"and the run link is omitted when there is none": {
			status:    commentRulesFailed,
			wantSteps: false,
			wantLink:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := formatCommentBody("vmphase", tc.status, tc.linkURL)

			if !strings.HasPrefix(body, "@vmphase, ") {
				t.Errorf("body does not open by addressing the author: %q", body)
			}

			if steps := strings.Contains(body, "\n1. "); steps != tc.wantSteps {
				t.Errorf("body lists steps = %t, want %t: %q", steps, tc.wantSteps, body)
			}

			if link := strings.Contains(body, testLinkURL); link != tc.wantLink {
				t.Errorf("body carries the link = %t, want %t: %q", link, tc.wantLink, body)
			}
		})
	}
}
