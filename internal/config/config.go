/*
[2026-07-31] :: 🚀 :: Added NetDNS (env NET_DNS, default 127.0.0.1:53) for configurable server egress DNS
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
	"strings"
	"time"

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
	NetDNS           string `env:"NET_DNS"             envDefault:"127.0.0.1:53"`
	// AwgIface overrides the AmneziaWG interface used for the awg route.
	// Empty = auto-detect: the single amneziawg interface (UCI proto) is used;
	// with several interfaces it must be set explicitly.
	AwgIface string `env:"AWG_IFACE" envDefault:""`
	// AwgHandshakeMax is the max age (seconds) of the latest AWG handshake
	// for the interface to be considered online.
	AwgHandshakeMax int `env:"AWG_HANDSHAKE_MAX" envDefault:"180"`
	// DefaultMode is the standing route when direct internet is available:
	// "awg" or "direct". "awg" falls back to direct when awg is offline.
	DefaultMode string `env:"DEFAULT_MODE" envDefault:"direct"`
	// ProbeURL is the site probed to detect direct internet presence.
	// Empty disables the probe. A 2xx/3xx response means direct access works.
	ProbeURL string `env:"PROBE_URL" envDefault:"https://www.google.com/generate_204"`
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

// to a sane 180s default when the value is missing or non-positive.

// AwgHandshakeMaxAge returns the handshake-freshness threshold for the awg route.
func (c *Config) AwgHandshakeMaxAge() time.Duration {
	if c.AwgHandshakeMax <= 0 {
		return 180 * time.Second
	}
	return time.Duration(c.AwgHandshakeMax) * time.Second
}

// ProbeEnabled reports whether the direct-internet probe is enabled.
func (c *Config) ProbeEnabled() bool {
	return c.ProbeURL != ""
}

// AwgPreferred reports whether awg is the configured standing route
// (DEFAULT_MODE=awg, case-insensitive).
func (c *Config) AwgPreferred() bool {
	return strings.EqualFold(c.DefaultMode, "awg")
}
