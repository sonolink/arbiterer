package github

import (
	"context"
	"fmt"
	"strconv"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/sonolink/arbiterer/internal/config"
)

const (
	issuerURL = "https://token.actions.githubusercontent.com"
	jwksURL   = issuerURL + "/.well-known/jwks"
)

// Verifier checks OIDC tokens GitHub Actions issues to workflow runs.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewVerifier builds a Verifier that trusts tokens issued by GitHub Actions for the
// configured audience.
func NewVerifier(ctx context.Context, cfg config.GitHub) *Verifier {
	keySet := oidc.NewRemoteKeySet(ctx, jwksURL)
	verifier := oidc.NewVerifier(
		issuerURL,
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
	RepositoryID int64
}

// tokenClaims mirrors the claims GitHub Actions puts in an OIDC token. Numeric ids
// arrive as strings.
type tokenClaims struct {
	RepositoryID string `json:"repository_id"`
}

func (tc tokenClaims) claims() (*Claims, error) {
	repositoryID, err := strconv.ParseInt(tc.RepositoryID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("github: parsing repository id %q: %w", tc.RepositoryID, err)
	}

	return &Claims{RepositoryID: repositoryID}, nil
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
