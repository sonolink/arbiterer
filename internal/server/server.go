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
	sealer        *secrets.Sealer
	verifier      *github.Verifier
	githubApp     *github.AppClient
}

// New builds a Server from its configuration and dependencies.
func New(
	cfg config.Server,
	logger *slog.Logger,
	store *storage.Store,
	discordClient *discord.Client,
	sealer *secrets.Sealer,
	verifier *github.Verifier,
	githubApp *github.AppClient,
) *Server {
	return &Server{
		cfg:           cfg,
		logger:        logger,
		store:         store,
		discordClient: discordClient,
		sealer:        sealer,
		verifier:      verifier,
		githubApp:     githubApp,
	}
}

func (s *Server) addRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/resolve", s.handleResolve)
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
