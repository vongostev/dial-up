/*
[2026-07-10] :: 🚀 :: Added SocksProxyAddr/Port/User/Pass env fields (server-only SOCKS5 egress) + ErrInvalidSocksPort validation
[2026-07-09] :: 🚀 :: Added StatusPort (env STATUS_PORT, default 9091) for the local status HTTP endpoint
[2026-07-02] :: 🚀 :: Initial config package
*/

// Package config provides environment-based configuration.
package config

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/caarlos0/env/v11"
)

var ErrPlaceholderValue = errors.New("field is empty or has placeholder value")

// ErrInvalidSocksPort signals SOCKS_PROXY_PORT is non-numeric or out of range when SOCKS_PROXY_ADDR is set.
var ErrInvalidSocksPort = errors.New("SOCKS_PROXY_PORT must be a number 1..65535 when SOCKS_PROXY_ADDR is set")

// Config holds all environment-based configuration parameters for the bot.
type Config struct {
	VKToken          string `env:"VK_TOKEN,required"`
	OlcrtcKey        string `env:"OLCRTC_KEY,required"`
	IsClient         bool   `env:"IS_CLIENT"           envDefault:"true"`
	Debug            bool   `env:"DEBUG"               envDefault:"false"`
	OlcrtcExe        string `env:"OLCRTC_EXE"          envDefault:"/etc/olcrtc-linux-arm64"`
	DataDir          string `env:"DATA_DIR"            envDefault:"data"`
	LastProviderFile string `env:"LAST_PROVIDER_FILE"  envDefault:"last_provider.json"`
	AllowedUserIDs   string `env:"ALLOWED_USER_IDS"    envDefault:""`
	SleepOnError     int    `env:"SLEEP_ON_ERROR"      envDefault:"5"`
	StatusPort       string `env:"STATUS_PORT"         envDefault:"9091"`
	SocksProxyAddr   string `env:"SOCKS_PROXY_ADDR"    envDefault:""`
	SocksProxyPort   string `env:"SOCKS_PROXY_PORT"    envDefault:""`
	SocksProxyUser   string `env:"SOCKS_PROXY_USER"    envDefault:""`
	SocksProxyPass   string `env:"SOCKS_PROXY_PASS"    envDefault:""`
}

// Load parses environment variables into a Config struct.
func Load() (*Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, err
	}

	// Ensure tokens are not empty or placeholders [Access denied: token required]
	if cfg.VKToken == "" || cfg.VKToken == "your_vk_user_token_here" {
		return nil, fmt.Errorf("VK_TOKEN %w", ErrPlaceholderValue)
	}
	if cfg.OlcrtcKey == "" || cfg.OlcrtcKey == "your_openssl_rand_hex_32_here" {
		return nil, fmt.Errorf("OLCRTC_KEY %w", ErrPlaceholderValue)
	}

	if err := cfg.validateSocksPort(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateSocksPort checks SOCKS_PROXY_PORT validity when the upstream proxy is enabled.
func (c *Config) validateSocksPort() error {
	if c.SocksProxyAddr == "" {
		return nil
	}
	port, err := strconv.Atoi(c.SocksProxyPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("SOCKS_PROXY_PORT %w", ErrInvalidSocksPort)
	}
	return nil
}
