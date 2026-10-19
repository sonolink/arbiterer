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

package config

import (
	"bytes"
	"crypto/rsa"
	"encoding"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/golang-jwt/jwt/v5"
)

// Config holds all runtime settings for the application.
type Config struct {
	Log      Log
	Discord  Discord
	Postgres Postgres
	Server   Server
	Secrets  Secrets
	GitHub   GitHub
}

// Load reads the full configuration from environment variables and Docker
// secret files.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}

	if err := cfg.loadSecrets(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// loadSecrets fills the fields provided as Docker secrets.
func (c *Config) loadSecrets() error {
	secrets := []struct {
		name string
		dst  encoding.TextUnmarshaler
	}{
		{"POSTGRES_PASSWORD", &c.Postgres.Password},
		{"DISCORD_CLIENT_SECRET", &c.Discord.ClientSecret},
		{"GITHUB_CLIENT_SECRET", &c.GitHub.ClientSecret},
		{"GITHUB_CLIENT_PRIVATE_KEY", &c.GitHub.PrivateKey},
		{"GITHUB_WEBHOOK_SECRET", &c.GitHub.WebhookSecret},
		{"STORAGE_ENCRYPTION_KEY", &c.Secrets.StorageKey},
		{"BROWSER_ENCRYPTION_KEY", &c.Secrets.BrowserKey},
	}

	for _, s := range secrets {
		if err := decodeSecret(s.name, s.dst); err != nil {
			return err
		}
	}

	return nil
}

// LogFormat is the format used for log output.
type LogFormat string

const (
	// LogFormatText writes logs as plain text.
	LogFormatText LogFormat = "text"
	// LogFormatJSON writes logs as JSON.
	LogFormatJSON LogFormat = "json"
)

// UnmarshalText parses text or the json log format.
func (f *LogFormat) UnmarshalText(text []byte) error {
	switch format := LogFormat(text); format {
	case LogFormatText, LogFormatJSON:
		*f = format

		return nil
	default:
		return fmt.Errorf(
			"invalid log format %q, want %q or %q",
			format,
			LogFormatText,
			LogFormatJSON,
		)
	}
}

// Log holds the settings that control how the app writes logs.
type Log struct {
	Format LogFormat  `env:"LOG_FORMAT" envDefault:"text"`
	Level  slog.Level `env:"LOG_LEVEL"  envDefault:"info"`
}

// Handler builds a slog handler that writes to w using the configured
// format and level.
func (l Log) Handler(w io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: l.Level}

	switch l.Format {
	case LogFormatJSON:
		return slog.NewJSONHandler(w, opts)
	default:
		return slog.NewTextHandler(w, opts)
	}
}

// LoadLog reads only the logging settings from the environment.
func LoadLog() (Log, error) {
	cfg, err := env.ParseAs[Log]()
	if err != nil {
		return Log{}, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
}

// Discord holds application settings used for OAuth.
type Discord struct {
	ClientID     string `env:"DISCORD_CLIENT_ID,required"`
	ClientSecret Secret `env:"-"`
	RedirectURI  string `env:"DISCORD_REDIRECT_URI,required"`
	APIURL       string `env:"DISCORD_API_URL" envDefault:"https://discord.com/api/v10"`
}

// Postgres holds the configuration to connect to the postgres database.
type Postgres struct {
	User     string `env:"POSTGRES_USER,required"`
	Password Secret `env:"-"`
	Host     string `env:"POSTGRES_HOST,required"`
	Port     string `env:"POSTGRES_PORT" envDefault:"5432"`
	Name     string `env:"POSTGRES_DB,required"`
	SSLMode  string `env:"POSTGRES_SSLMODE" envDefault:"disable"`
}

// DSN builds a Postgres connection string from the settings.
func (p Postgres) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, string(p.Password)),
		Host:     net.JoinHostPort(p.Host, p.Port),
		Path:     "/" + p.Name,
		RawQuery: url.Values{"sslmode": {p.SSLMode}}.Encode(),
	}
	return u.String()
}

// Server holds the HTTP listener settings.
type Server struct {
	Host               string        `env:"SERVER_HOST"             envDefault:"127.0.0.1"`
	Port               int           `env:"SERVER_PORT"             envDefault:"8080"`
	ReadTimeout        time.Duration `env:"SERVER_READ_TIMEOUT"     envDefault:"5s"`
	WriteTimeout       time.Duration `env:"SERVER_WRITE_TIMEOUT"    envDefault:"30s"`
	IdleTimeout        time.Duration `env:"SERVER_IDLE_TIMEOUT"     envDefault:"120s"`
	ShutdownTimeout    time.Duration `env:"SERVER_SHUTDOWN_TIMEOUT" envDefault:"10s"`
	PublicURL          string        `env:"SERVER_PUBLIC_URL,required"`
	LinkTokenLifetime  time.Duration `env:"LINK_TOKEN_LIFETIME" envDefault:"15m"`
	LinkCookieLifetime time.Duration `env:"LINK_COOKIE_LIFETIME" envDefault:"10m"`
}

// Addr combines host and port into a listener address.
func (s Server) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// LoadPostgres reads only the Postgres settings, without the rest of the app config.
func LoadPostgres() (Postgres, error) {
	cfg, err := env.ParseAs[Postgres]()
	if err != nil {
		return Postgres{}, fmt.Errorf("config: %w", err)
	}

	if err := decodeSecret("POSTGRES_PASSWORD", &cfg.Password); err != nil {
		return Postgres{}, err
	}

	return cfg, nil
}

// keySize is the required length in bytes of a Key.
const keySize = 32

// Key is a 32 byte secret, decoded from a base64 encoded value.
type Key []byte

// UnmarshalText decodes a base64 encoded key and rejects it unless it is keySize bytes long.
func (k *Key) UnmarshalText(text []byte) error {
	key, err := base64.StdEncoding.DecodeString(string(text))
	if err != nil {
		return fmt.Errorf("invalid base64: %w", err)
	}

	if len(key) != keySize {
		return fmt.Errorf("key must be %d bytes, got %d", keySize, len(key))
	}

	*k = key

	return nil
}

// Secrets holds the keys used to encrypt sensitive values.
type Secrets struct {
	StorageKey Key `env:"-"`
	BrowserKey Key `env:"-"`
}

// GitHub holds the application settings used for OAuth and OIDC.
type GitHub struct {
	ClientID      string        `env:"GITHUB_CLIENT_ID,required"`
	ClientSecret  Secret        `env:"-"`
	PrivateKey    RSAPrivateKey `env:"-"`
	WebhookSecret Secret        `env:"-"`
	RedirectURI   string        `env:"GITHUB_REDIRECT_URI,required"`
	OIDCAudience  string        `env:"GITHUB_OIDC_AUDIENCE,required"`
	APIURL        string        `env:"GITHUB_API_URL"     envDefault:"https://api.github.com"`
	OAuthURL      string        `env:"GITHUB_OAUTH_URL"   envDefault:"https://github.com"`
	OIDCIssuerURL string        `env:"GITHUB_OIDC_ISSUER" envDefault:"https://token.actions.githubusercontent.com"`
}

// RSAPrivateKey is a GitHub App's RSA private key, decoded from a PEM encoded
// value, optionally base64 encoded to keep it on a single line.
type RSAPrivateKey rsa.PrivateKey

// UnmarshalText parses a PEM encoded RSA private key, base64 decoding it
// first unless it already starts with a PEM header.
func (k *RSAPrivateKey) UnmarshalText(text []byte) error {
	pemBytes := bytes.TrimSpace(text)
	if !bytes.HasPrefix(pemBytes, []byte("-----BEGIN")) {
		decoded, err := base64.StdEncoding.DecodeString(string(pemBytes))
		if err != nil {
			return fmt.Errorf("invalid private key: not PEM and invalid base64: %w", err)
		}

		pemBytes = decoded
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	*k = RSAPrivateKey(*key)

	return nil
}
