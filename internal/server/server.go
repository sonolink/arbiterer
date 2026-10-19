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
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sonolink/arbiterer/internal/config"
	"github.com/sonolink/arbiterer/internal/discord"
	"github.com/sonolink/arbiterer/internal/github"
	"github.com/sonolink/arbiterer/internal/secrets"
	"github.com/sonolink/arbiterer/internal/storage"
)

// Server runs the HTTP service with the dependencies it needs to handle requests.
type Server struct {
	cfg           config.Server
	logger        *slog.Logger
	store         *storage.Store
	discordClient *discord.Client
	githubClient  *github.Client
	storageSealer *secrets.Sealer
	browserSealer *secrets.Sealer
	verifier      *github.Verifier
}

// New builds a Server from its configuration and dependencies.
func New(
	cfg config.Server,
	logger *slog.Logger,
	store *storage.Store,
	discordClient *discord.Client,
	githubClient *github.Client,
	storageSealer *secrets.Sealer,
	browserSealer *secrets.Sealer,
	verifier *github.Verifier,
) *Server {
	return &Server{
		cfg:           cfg,
		logger:        logger,
		store:         store,
		discordClient: discordClient,
		githubClient:  githubClient,
		storageSealer: storageSealer,
		browserSealer: browserSealer,
		verifier:      verifier,
	}
}

func (s *Server) addRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/discord/resolve", s.handleDiscordResolve)
	mux.HandleFunc("POST /v1/discord/member", s.handleDiscordMember)
	mux.HandleFunc("POST /v1/github/pulls/close", s.handleGitHubPullRequestClose)
	mux.HandleFunc("POST /v1/github/pulls/passed", s.handleGitHubPullRequestPassed)
	mux.HandleFunc("POST /v1/github/pulls/rules-failed", s.handleGitHubPullRequestRulesFailed)
	mux.HandleFunc("POST /v1/github/pulls/open", s.handleGitHubPullRequestOpen)
	mux.HandleFunc("POST /v1/github/webhooks", s.handleGitHubWebhook)
	mux.HandleFunc("GET /v1/link", s.handleLink)
	mux.HandleFunc("GET /v1/link/github/callback", s.handleLinkGitHubCallback)
	mux.HandleFunc("GET /v1/link/discord", s.handleLinkDiscord)
	mux.HandleFunc("GET /v1/link/discord/callback", s.handleLinkDiscordCallback)
}

// Run starts the HTTP server and blocks until it stops, draining in-flight
// requests during shutdown.
func (s *Server) Run() error {
	mux := http.NewServeMux()
	s.addRoutes(mux)

	srv := &http.Server{
		Addr:         s.cfg.Addr(),
		Handler:      mux,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}

	s.logger.Info("starting server", "addr", srv.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	quitCh := make(chan os.Signal, 1)
	signal.Notify(quitCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-quitCh:
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		s.cfg.ShutdownTimeout,
	)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return err
	}

	s.logger.Info("server has now shutdown...")
	return <-errCh
}
