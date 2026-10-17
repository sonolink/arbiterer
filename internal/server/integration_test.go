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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sonolink/arbiterer/internal/config"
	"github.com/sonolink/arbiterer/internal/discord"
	"github.com/sonolink/arbiterer/internal/github"
	"github.com/sonolink/arbiterer/internal/secrets"
	"github.com/sonolink/arbiterer/internal/storage"
)

const (
	// postgresImage is pinned to the image compose runs, so a query that works
	// here works there.
	postgresImage = "postgres:18.6-alpine"

	// testAudience is what the app requires of a token's aud claim.
	testAudience = "arbiterer"

	// keyID names the signing key in both the token header and the published set.
	keyID = "test-key"
)

// testStore is shared by every test in this package. Starting a container per
// test would cost ci time.
var testStore *storage.Store

// ids hands out values no other test is using. Rows are keyed by the repository
// a test supplies, so tests that keep to their own ids stay independent and can
// run in parallel.
var ids atomic.Int64

// nextID returns an id reserved for the calling test.
func nextID() int64 {
	return ids.Add(1)
}

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(code)
}

// run owns the container for the length of the package's tests. It is separate
// from TestMain so that the cleanup still happens on the way out, which an
// os.Exit would skip.
func run(m *testing.M) (int, error) {
	ctx := context.Background()

	container, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("arbiterer"),
		postgres.WithUsername("arbiterer"),
		postgres.WithPassword("arbiterer"),
		postgres.BasicWaitStrategies(),
	)

	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintf(os.Stderr, "terminating postgres: %v\n", err)
		}
	}()

	if err != nil {
		return 0, fmt.Errorf("starting postgres: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, fmt.Errorf("building the connection string: %w", err)
	}

	store, err := storage.NewStore(ctx, dsn)
	if err != nil {
		return 0, fmt.Errorf("opening the store: %w", err)
	}

	defer store.Close()

	// Running the real migrations is half the point: it is the only thing that
	// checks they apply to an empty database at the version we ship.
	if err := store.Migrate(ctx); err != nil {
		return 0, fmt.Errorf("migrating: %w", err)
	}

	testStore = store

	return m.Run(), nil
}

// oidcIssuer stands in for GitHub's Actions token service, so the signature,
// issuer and audience checks all run against a key the test controls.
type oidcIssuer struct {
	url string
	key *rsa.PrivateKey
}

func newOIDCIssuer(t *testing.T) *oidcIssuer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a signing key: %v", err)
	}

	idp := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{
			{PublicKey: key.Public(), KeyID: keyID, Algorithm: oidc.RS256},
		},
	}

	// oidctest publishes the keys at /keys, GitHub publishes them at the path
	// the verifier asks for.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/jwks", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/keys"
		idp.ServeHTTP(w, r)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	idp.SetIssuer(server.URL)

	return &oidcIssuer{url: server.URL, key: key}
}

// token mints a token carrying the claims a workflow run would present.
func (i *oidcIssuer) token(t *testing.T, claims github.Claims) string {
	t.Helper()

	return oidctest.SignIDToken(i.key, keyID, oidc.RS256, fmt.Sprintf(`{
		"iss": %q,
		"aud": %q,
		"sub": "repo:%s:ref:refs/heads/main",
		"exp": %d,
		"repository": %q,
		"repository_id": "%d",
		"repository_owner_id": "%d",
		"run_id": "%d",
		"check_run_id": "%d"
	}`,
		i.url,
		testAudience,
		claims.Repository,
		time.Now().Add(time.Minute).Unix(),
		claims.Repository,
		claims.RepositoryID,
		claims.RepositoryOwnerID,
		claims.RunID,
		claims.CheckRunID,
	))
}

// recordedRequest is one call the app made to a stand in.
type recordedRequest struct {
	Method string
	Path   string
}

// fakeGitHub answers the calls the app makes to the GitHub API and records
// them, so a flow can be checked by what it tried to change.
type fakeGitHub struct {
	url string

	mu       sync.Mutex
	requests []recordedRequest
}

const (
	// commentID is the id the stand in gives every comment it creates.
	commentID = 9001

	// issueAuthor is the login it reports for every pull request.
	issueAuthor = "vmphase"
)

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()

	gh := &fakeGitHub{}

	mux := http.NewServeMux()

	// Every write goes through these two: find the installation, then mint a
	// token for it.
	mux.HandleFunc("GET /repos/{owner}/{repo}/installation", respond(`{"id":1}`))
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", respond(`{"token":"ghs-test"}`))

	mux.HandleFunc(
		"GET /repos/{owner}/{repo}/issues/{number}",
		respond(fmt.Sprintf(`{"user":{"login":%q}}`, issueAuthor)),
	)
	mux.HandleFunc(
		"POST /repos/{owner}/{repo}/issues/{number}/comments",
		respond(fmt.Sprintf(`{"id":%d}`, commentID)),
	)

	// A call to a path that is not set up is a mistake worth seeing, rather
	// than an empty object that fails somewhere later.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected call to the github stand in: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotImplemented)
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gh.mu.Lock()
		gh.requests = append(gh.requests, recordedRequest{Method: r.Method, Path: r.URL.Path})
		gh.mu.Unlock()

		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	gh.url = server.URL

	return gh
}

// called reports whether the app made the given call.
func (g *fakeGitHub) called(method, path string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	return slices.Contains(g.requests, recordedRequest{Method: method, Path: path})
}

// respond answers with a fixed JSON body.
func respond(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

// harness is everything a flow test drives, the app under test, the database it
// writes to, and the stand ins it talks to.
type harness struct {
	url    string
	issuer *oidcIssuer
	github *fakeGitHub
}

// newHarness builds a Server wired to the shared database and to stand ins for
// GitHub, and serves it.
func newHarness(t *testing.T) *harness {
	t.Helper()

	issuer := newOIDCIssuer(t)
	gh := newFakeGitHub(t)

	// The app signs its own JWT before it can ask for an installation token, so
	// a usable key has to be here even though the stand in never checks it.
	appKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating an app key: %v", err)
	}

	githubCfg := config.GitHub{
		ClientID:      "github-client-id",
		PrivateKey:    config.RSAPrivateKey(*appKey),
		RedirectURI:   "https://arbiterer.test/v1/link/github/callback",
		OIDCAudience:  testAudience,
		APIURL:        gh.url,
		OAuthURL:      gh.url,
		OIDCIssuerURL: issuer.url,
	}

	storageSealer, err := secrets.NewSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	browserSealer, err := secrets.NewSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	s := New(
		config.Server{
			PublicURL:          "https://arbiterer.test",
			LinkTokenLifetime:  15 * time.Minute,
			LinkCookieLifetime: 10 * time.Minute,
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testStore,
		discord.NewClient(config.Discord{}),
		github.NewClient(githubCfg),
		storageSealer,
		browserSealer,
		github.NewVerifier(t.Context(), githubCfg),
	)

	mux := http.NewServeMux()
	s.addRoutes(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &harness{url: server.URL, issuer: issuer, github: gh}
}

// post sends a request to the app with the given bearer token, and returns the
// status and body. Reading the body here keeps its lifetime out of the tests.
func (h *harness) post(t *testing.T, path, token, body string) (status int, response []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		h.url+path,
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}

	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}

	return resp.StatusCode, got
}
