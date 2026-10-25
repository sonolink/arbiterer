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
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sonolink/arbiterer/internal/config"
	"github.com/sonolink/arbiterer/internal/github"
)

const (
	testWorkflowPath      = ".github/workflows/arbiterer.yml"
	testWorkflowSHA       = "0123456789abcdef0123456789abcdef01234567"
	testDefaultBranch     = "main"
	testInstallationToken = "installation-token"
)

// testRulesSource is a workflow fragment with an inline rules block.
const testRulesSource = `rules: |
  if (!user.mfa_enabled) {
    core.setFailed("Enable two-factor authentication.");
  }
`

// TestWorkflowRunURL covers the link back to the run. The ids are optional
// claims, so every combination of them missing has to produce something a
// comment can carry, including nothing at all.
func TestWorkflowRunURL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		claims github.Claims
		want   string
	}{
		"a run with a job links straight to the job": {
			claims: github.Claims{
				Repository: testRepository,
				RunID:      15,
				CheckRunID: 42,
			},
			want: "https://github.com/sonolink/arbiterer/actions/runs/15/job/42",
		},
		"a run without a job links to the run": {
			claims: github.Claims{
				Repository: testRepository,
				RunID:      15,
			},
			want: "https://github.com/sonolink/arbiterer/actions/runs/15",
		},
		"no run means no link": {
			claims: github.Claims{Repository: testRepository},
			want:   "",
		},
		"and a job without its run is not enough": {
			claims: github.Claims{
				Repository: testRepository,
				CheckRunID: 42,
			},
			want: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := workflowRunURL(&tc.claims); got != tc.want {
				t.Errorf("workflowRunURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestWorkflowFile covers which workflow a rules link may point at. A claim that
// cannot be trusted is rejected rather than guessed at.
func TestWorkflowFile(t *testing.T) {
	t.Parallel()

	ref := testRepository + "/" + testWorkflowPath + "@refs/heads/" + testDefaultBranch

	tests := map[string]struct {
		claims   github.Claims
		wantPath string
		wantRef  string
		wantOK   bool
	}{
		"the commit the workflow came from is preferred": {
			claims:   github.Claims{Repository: testRepository, WorkflowRef: ref, WorkflowSHA: testWorkflowSHA},
			wantPath: testWorkflowPath,
			wantRef:  testWorkflowSHA,
			wantOK:   true,
		},
		"without a commit the branch is used": {
			claims:   github.Claims{Repository: testRepository, WorkflowRef: ref},
			wantPath: testWorkflowPath,
			wantRef:  testDefaultBranch,
			wantOK:   true,
		},
		"a branch keeps the slashes in its name": {
			claims:   github.Claims{Repository: testRepository, WorkflowRef: testRepository + "/" + testWorkflowPath + "@refs/heads/feature/rules"},
			wantPath: testWorkflowPath,
			wantRef:  "feature/rules",
			wantOK:   true,
		},
		"a branch containing an at sign still finds the commit": {
			claims: github.Claims{
				Repository:  testRepository,
				WorkflowRef: testRepository + "/" + testWorkflowPath + "@refs/heads/fix@2",
				WorkflowSHA: testWorkflowSHA,
			},
			wantPath: testWorkflowPath,
			wantRef:  testWorkflowSHA,
			wantOK:   true,
		},
		"a malformed commit falls back to the branch": {
			claims:   github.Claims{Repository: testRepository, WorkflowRef: ref, WorkflowSHA: "0123456789abcdef0123456789abcdef0123456z"},
			wantPath: testWorkflowPath,
			wantRef:  testDefaultBranch,
			wantOK:   true,
		},
		"a yaml workflow is accepted": {
			claims:   github.Claims{Repository: testRepository, WorkflowRef: testRepository + "/.github/workflows/ci.yaml@refs/heads/" + testDefaultBranch},
			wantPath: ".github/workflows/ci.yaml",
			wantRef:  testDefaultBranch,
			wantOK:   true,
		},
		"a workflow from another repository is not linked": {
			claims: github.Claims{Repository: testRepository, WorkflowRef: "someone/else/" + testWorkflowPath + "@refs/heads/" + testDefaultBranch},
		},
		"a pull request ref is not the branch that ran": {
			claims: github.Claims{Repository: testRepository, WorkflowRef: testRepository + "/" + testWorkflowPath + "@refs/pull/12/merge"},
		},
		"a file outside the workflows directory is not linked": {
			claims: github.Claims{Repository: testRepository, WorkflowRef: testRepository + "/ci.yml@refs/heads/" + testDefaultBranch},
		},
		"a path that climbs out is not linked": {
			claims: github.Claims{Repository: testRepository, WorkflowRef: testRepository + "/.github/workflows/../../etc/passwd.yml@refs/heads/" + testDefaultBranch},
		},
		"a token without a workflow claim links nothing": {
			claims: github.Claims{Repository: testRepository},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path, gotRef, ok := workflowFile(&tc.claims)
			if ok != tc.wantOK {
				t.Fatalf("ok = %t, want %t (path %q, ref %q)", ok, tc.wantOK, path, gotRef)
			}

			if !ok {
				return
			}

			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}

			if gotRef != tc.wantRef {
				t.Errorf("ref = %q, want %q", gotRef, tc.wantRef)
			}
		})
	}
}

// TestRulesLines covers the range the comment highlights, which is the inline
// rules block and nothing around it.
func TestRulesLines(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		source    string
		wantStart int
		wantEnd   int
		wantOK    bool
	}{
		"an inline block is anchored to its own lines": {
			source:    testRulesSource,
			wantStart: 2,
			wantEnd:   4,
			wantOK:    true,
		},
		"a folded scalar is a block too": {
			source:    strings.Replace(testRulesSource, "rules: |", "rules: >-", 1),
			wantStart: 2,
			wantEnd:   4,
			wantOK:    true,
		},
		"an expression is not a block": {
			source: "rules: ${{ steps.rules.outputs.source }}\n",
		},
		"an empty first block is not skipped": {
			source: "rules: |\nrules: |\n  core.setFailed()\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			start, end, ok := rulesLines(tc.source)
			if ok != tc.wantOK {
				t.Fatalf("rulesLines = %d-%d, %t, want ok %t", start, end, ok, tc.wantOK)
			}

			if !ok {
				return
			}

			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("range = %d-%d, want %d-%d", start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

// TestRulesFileURL covers the link the comment carries and how it degrades when
// the workflow cannot be read or has no rules block.
func TestRulesFileURL(t *testing.T) {
	t.Parallel()

	claims := github.Claims{
		Repository:  testRepository,
		WorkflowRef: testRepository + "/" + testWorkflowPath + "@refs/heads/" + testDefaultBranch,
	}

	pinned := claims
	pinned.WorkflowSHA = testWorkflowSHA

	base := "https://github.com/" + testRepository + "/blob/" + testDefaultBranch + "/" + testWorkflowPath

	tests := map[string]struct {
		claims  github.Claims
		token   string
		status  int
		body    string
		want    string
		wantRef string
	}{
		"the rules block is linked at the branch that ran": {
			claims:  claims,
			token:   testInstallationToken,
			status:  http.StatusOK,
			body:    contentsResponse(testRulesSource),
			want:    base + "#L2-L4",
			wantRef: testDefaultBranch,
		},
		"and pinned to the commit when the run carries one": {
			claims:  pinned,
			token:   testInstallationToken,
			status:  http.StatusOK,
			body:    contentsResponse(testRulesSource),
			want:    "https://github.com/" + testRepository + "/blob/" + testWorkflowSHA + "/" + testWorkflowPath + "#L2-L4",
			wantRef: testWorkflowSHA,
		},
		"a file without a block is linked without an anchor": {
			claims:  claims,
			token:   testInstallationToken,
			status:  http.StatusOK,
			body:    contentsResponse("name: CI\n"),
			want:    base,
			wantRef: testDefaultBranch,
		},
		"an unreadable file is linked without an anchor": {
			claims:  claims,
			token:   testInstallationToken,
			status:  http.StatusNotFound,
			body:    `{"message":"Not Found"}`,
			want:    base,
			wantRef: testDefaultBranch,
		},
		"without a token the file is linked but not read": {
			claims: claims,
			want:   base,
		},
		"a branch with url characters is escaped": {
			claims: github.Claims{
				Repository:  testRepository,
				WorkflowRef: testRepository + "/" + testWorkflowPath + "@refs/heads/fix#1)/part",
			},
			want: "https://github.com/" + testRepository + "/blob/fix%231%29/part/" + testWorkflowPath,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				mu       sync.Mutex
				gotPath  string
				gotQuery string
			)

			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				mu.Unlock()

				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(api.Close)

			s := &Server{
				logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
				githubClient: github.NewClient(config.GitHub{APIURL: api.URL}),
			}

			if got := s.rulesFileURL(t.Context(), tc.token, &tc.claims); got != tc.want {
				t.Errorf("rulesFileURL = %q, want %q", got, tc.want)
			}

			mu.Lock()
			path, query := gotPath, gotQuery
			mu.Unlock()

			if tc.wantRef == "" {
				if path != "" {
					t.Errorf("the app read %q, which it should not", path)
				}

				return
			}

			wantPath := "/repos/" + testRepository + "/contents/" + testWorkflowPath
			if path != wantPath {
				t.Errorf("read %q, want %q", path, wantPath)
			}

			if want := "ref=" + tc.wantRef; query != want {
				t.Errorf("query = %q, want %q", query, want)
			}
		})
	}
}

// contentsResponse wraps a file the way the GitHub contents API returns it.
func contentsResponse(source string) string {
	return fmt.Sprintf(
		`{"type":"file","encoding":"base64","content":%q}`,
		base64.StdEncoding.EncodeToString([]byte(source)),
	)
}
