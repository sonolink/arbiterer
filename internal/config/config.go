package config

import (
	"crypto/rsa"
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

// Load reads the full configuration from environment variables.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
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
	ClientSecret string `env:"DISCORD_CLIENT_SECRET,required"`
	RedirectURI  string `env:"DISCORD_REDIRECT_URI,required"`
}

// Postgres holds the configuration to connect to the postgres database.
type Postgres struct {
	User     string `env:"POSTGRES_USER,required"`
	Password string `env:"POSTGRES_PASSWORD,required"`
	Host     string `env:"POSTGRES_HOST,required"`
	Port     string `env:"POSTGRES_PORT" envDefault:"5432"`
	Name     string `env:"POSTGRES_DB,required"`
	SSLMode  string `env:"POSTGRES_SSLMODE" envDefault:"disable"`
}

// DSN builds a Postgres connection string from the settings.
func (p Postgres) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
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

	return cfg, nil
}

// keySize is the required length in bytes of a Key.
const keySize = 32

// Key is a 32 byte secret, decoded from a base64 encoded environment value.
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

// Secrets holds the keys used to encrypt secrets at rest.
type Secrets struct {
	TokenKey  Key `env:"TOKEN_ENCRYPTION_KEY,required"`
	CookieKey Key `env:"COOKIE_SIGNING_KEY,required"`
}

// GitHub holds the application settings used for OAuth and OIDC.
type GitHub struct {
	ClientID     string     `env:"GITHUB_CLIENT_ID,required"`
	ClientSecret string     `env:"GITHUB_CLIENT_SECRET,required"`
	PrivateKey   PrivateKey `env:"GITHUB_CLIENT_PRIVATE_KEY,required"`
	RedirectURI  string     `env:"GITHUB_REDIRECT_URI,required"`
	OIDCAudience string     `env:"GITHUB_OIDC_AUDIENCE,required"`
}

// PrivateKey is a GitHub App's RSA private key, decoded from a PEM encoded
// environment value.
type PrivateKey rsa.PrivateKey

// UnmarshalText parses a PEM encoded RSA private key.
func (k *PrivateKey) UnmarshalText(text []byte) error {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(text)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	*k = PrivateKey(*key)

	return nil
}
