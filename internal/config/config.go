package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/kirsle/configdir"
	"github.com/kkyr/fig"
)

// Config holds all configuration for the application
type Config struct {
	// Server configuration
	Addr string `fig:"addr" default:":8080"`

	// Database configuration
	DBPath string `fig:"db_path" default:"rssgrid.db"`

	// Feed configuration
	UpdateInterval time.Duration `fig:"update_interval" default:"30m"`

	// Session configuration
	SessionKey string `fig:"session_key" env:"RSSGRID_SESSION_KEY"`

	// OIDC configuration
	OIDC struct {
		IssuerURL    string `fig:"issuer_url" env:"RSSGRID_OIDC_ISSUER_URL"`
		ClientID     string `fig:"client_id" env:"RSSGRID_OIDC_CLIENT_ID"`
		ClientSecret string `fig:"client_secret" env:"RSSGRID_OIDC_CLIENT_SECRET"`
		RedirectURL  string `fig:"redirect_url" default:"http://localhost:8080/auth/callback"`
	} `fig:"oidc"`
}

// Load loads configuration from a JSON file and environment variables
func Load() (*Config, error) {
	// Get config directory using configdir
	configDir := configdir.LocalConfig("rssgrid")

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	configPath := filepath.Join(configDir, "rssgrid.json")

	var cfg Config

	// Load configuration using fig (JSON format)
	if err := fig.Load(&cfg,
		fig.File(configPath),
		fig.UseEnv("RSSGRID"),
	); err != nil {
		return nil, err
	}

	// Validate required fields
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate checks that all required configuration fields are set
func (c *Config) validate() error {
	if c.OIDC.IssuerURL == "" {
		return &ConfigError{Field: "oidc.issuer_url", Message: "OIDC issuer URL is required"}
	}
	if c.OIDC.ClientID == "" {
		return &ConfigError{Field: "oidc.client_id", Message: "OIDC client ID is required"}
	}
	if c.OIDC.ClientSecret == "" {
		return &ConfigError{Field: "oidc.client_secret", Message: "OIDC client secret is required"}
	}
	if c.SessionKey == "" {
		return &ConfigError{Field: "session_key", Message: "Session key is required"}
	}
	return nil
}

// ConfigError represents a configuration validation error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}
