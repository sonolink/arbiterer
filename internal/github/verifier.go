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

import (
	"context"
	"fmt"
	"strconv"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/sonolink/arbiterer/internal/config"
)

// jwksPath is where an issuer publishes the keys its tokens are signed with.
const jwksPath = "/.well-known/jwks"

// Verifier checks OIDC tokens GitHub Actions issues to workflow runs.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewVerifier builds a Verifier that trusts tokens issued by GitHub Actions for the
// configured audience.
func NewVerifier(ctx context.Context, cfg config.GitHub) *Verifier {
	keySet := oidc.NewRemoteKeySet(ctx, cfg.OIDCIssuerURL+jwksPath)
	verifier := oidc.NewVerifier(
		cfg.OIDCIssuerURL,
		keySet,
		&oidc.Config{
			ClientID:             cfg.OIDCAudience,
			SupportedSigningAlgs: []string{oidc.RS256},
		},
	)
	return &Verifier{verifier: verifier}
}

// Claims holds the parts of a verified token the application acts on.
type Claims struct {
	RepositoryID      int64
	RepositoryOwnerID int64
	Repository        string
	RunID             int64
	CheckRunID        int64
}

// tokenClaims mirrors the claims GitHub Actions puts in an OIDC token. Numeric ids
// arrive as strings.
type tokenClaims struct {
	RepositoryID      string `json:"repository_id"`
	RepositoryOwnerID string `json:"repository_owner_id"`
	Repository        string `json:"repository"`
	RunID             string `json:"run_id"`
	CheckRunID        string `json:"check_run_id"`
}

func (tc tokenClaims) claims() (*Claims, error) {
	repositoryID, err := strconv.ParseInt(tc.RepositoryID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("github: parsing repository id %q: %w", tc.RepositoryID, err)
	}

	ownerID, err := strconv.ParseInt(tc.RepositoryOwnerID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("github: parsing repository owner id %q: %w", tc.RepositoryOwnerID, err)
	}

	if tc.Repository == "" {
		return nil, fmt.Errorf("github: token missing repository claim")
	}

	runID, err := strconv.ParseInt(tc.RunID, 10, 64)
	if err != nil && tc.RunID != "" {
		return nil, fmt.Errorf("github: parsing run id %q: %w", tc.RunID, err)
	}

	checkRunID, err := strconv.ParseInt(tc.CheckRunID, 10, 64)
	if err != nil && tc.CheckRunID != "" {
		return nil, fmt.Errorf("github: parsing check run id %q: %w", tc.CheckRunID, err)
	}

	return &Claims{
		RepositoryID:      repositoryID,
		RepositoryOwnerID: ownerID,
		Repository:        tc.Repository,
		RunID:             runID,
		CheckRunID:        checkRunID,
	}, nil
}

// Verify checks a GitHub Actions OIDC token and returns the claims it carries.
func (v *Verifier) Verify(ctx context.Context, rawIDToken string) (*Claims, error) {
	idToken, err := v.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("github: verifying token: %w", err)
	}

	var tc tokenClaims
	if err := idToken.Claims(&tc); err != nil {
		return nil, fmt.Errorf("github: decoding claims: %w", err)
	}

	return tc.claims()
}
