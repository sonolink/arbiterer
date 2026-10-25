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

import "testing"

const (
	testRepository = "sonolink/arbiterer"
	testWorkflowSHA = "0123456789abcdef0123456789abcdef01234567"
)

// TestTokenClaims covers which claims a token has to carry. Links are scoped by
// repository id, so it has to fail rather than fall back to zero. run_id and
// check_run_id only build a URL back to the run, so they are allowed to be
// absent.
func TestTokenClaims(t *testing.T) {
	t.Parallel()

	full := tokenClaims{
		RepositoryID:      "1296269",
		RepositoryOwnerID: "583231",
		Repository:        testRepository,
		RunID:             "15",
		CheckRunID:        "42",
		WorkflowRef:       testRepository + "/.github/workflows/ci.yml@refs/heads/main",
		WorkflowSHA:       testWorkflowSHA,
	}

	// without returns a copy of full with one claim emptied.
	without := func(set func(tc *tokenClaims)) tokenClaims {
		tc := full
		set(&tc)

		return tc
	}

	tests := map[string]struct {
		claims  tokenClaims
		want    Claims
		wantErr bool
	}{
		"a complete token carries every id": {
			claims: full,
			want: Claims{
				RepositoryID:      1296269,
				RepositoryOwnerID: 583231,
				Repository:        testRepository,
				RunID:             15,
				CheckRunID:        42,
				WorkflowRef:       testRepository + "/.github/workflows/ci.yml@refs/heads/main",
				WorkflowSHA:       testWorkflowSHA,
			},
		},
		"a missing repository id is rejected": {
			claims:  without(func(tc *tokenClaims) { tc.RepositoryID = "" }),
			wantErr: true,
		},
		"a missing repository owner id is rejected": {
			claims:  without(func(tc *tokenClaims) { tc.RepositoryOwnerID = "" }),
			wantErr: true,
		},
		"a missing repository is rejected": {
			claims:  without(func(tc *tokenClaims) { tc.Repository = "" }),
			wantErr: true,
		},
		"a repository id that is not a number is rejected": {
			claims:  without(func(tc *tokenClaims) { tc.RepositoryID = testRepository }),
			wantErr: true,
		},
		"a missing run id is allowed and left at zero": {
			claims: without(func(tc *tokenClaims) { tc.RunID = "" }),
			want: Claims{
				RepositoryID:      1296269,
				RepositoryOwnerID: 583231,
				Repository:        testRepository,
				RunID:             0,
				CheckRunID:        42,
				WorkflowRef:       testRepository + "/.github/workflows/ci.yml@refs/heads/main",
				WorkflowSHA:       testWorkflowSHA,
			},
		},
		"a missing check run id is allowed and left at zero": {
			claims: without(func(tc *tokenClaims) { tc.CheckRunID = "" }),
			want: Claims{
				RepositoryID:      1296269,
				RepositoryOwnerID: 583231,
				Repository:        testRepository,
				RunID:             15,
				CheckRunID:        0,
				WorkflowRef:       testRepository + "/.github/workflows/ci.yml@refs/heads/main",
				WorkflowSHA:       testWorkflowSHA,
			},
		},
		"a missing workflow claim is allowed and left empty": {
			claims: without(func(tc *tokenClaims) {
				tc.WorkflowRef = ""
				tc.WorkflowSHA = ""
			}),
			want: Claims{
				RepositoryID:      1296269,
				RepositoryOwnerID: 583231,
				Repository:        testRepository,
				RunID:             15,
				CheckRunID:        42,
			},
		},
		"but a run id that is present and not a number is rejected": {
			claims:  without(func(tc *tokenClaims) { tc.RunID = "not-a-number" }),
			wantErr: true,
		},
		"as is a check run id that is present and not a number": {
			claims:  without(func(tc *tokenClaims) { tc.CheckRunID = "not-a-number" }),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.claims.claims()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("claims = %+v, want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("claims: %v", err)
			}

			if *got != tc.want {
				t.Errorf("claims = %+v, want %+v", *got, tc.want)
			}
		})
	}
}
