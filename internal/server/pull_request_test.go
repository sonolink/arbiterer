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
	"testing"

	"github.com/sonolink/arbiterer/internal/github"
)

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
