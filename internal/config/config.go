package config

import (
	"time"

	"github.com/kirsle/configdir"
	"github.com/kkyr/fig"
)

type Config struct {
	Addr           string        `fig:"addr" default:":8080"`
	DBPath         string        `fig:"db_path" default:"rssgrid.db"`
	UpdateInterval time.Duration `fig:"update_interval" default:"30m"`
	SessionKey     string        `fig:"session_key" env:"RSSGRID_SESSION_KEY" required:"true"`
	OIDC           struct {
		IssuerURL    string `fig:"issuer_url" env:"RSSGRID_OIDC_ISSUER_URL" required:"true"`
		ClientID     string `fig:"client_id" env:"RSSGRID_OIDC_CLIENT_ID" required:"true"`
		ClientSecret string `fig:"client_secret" env:"RSSGRID_OIDC_CLIENT_SECRET" required:"true"`
		RedirectURL  string `fig:"redirect_url" default:"http://localhost:8080/auth/callback"`
	} `fig:"oidc"`
}

func Load() (*Config, error) {
	configDir := configdir.LocalConfig("rssgrid")
	return LoadWithPath(configDir)
}

func LoadWithPath(configPath string) (*Config, error) {
	var cfg Config
	if err := fig.Load(&cfg,
		fig.File("rssgrid.json"),
		fig.Dirs(configPath),
		fig.UseEnv("RSSGRID"),
	); err != nil {
		return nil, err
	}
	return &cfg, nil
}
