package tests

import (
	"errors"
	"testing"

	"dial-up/internal/config"
)

// TestConfigSocksPortValidation validates the SOCKS proxy port field across enable/disable and boundary cases.
func TestConfigSocksPortValidation(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		port    string
		wantErr bool
	}{
		{"addr set + valid port", "203.0.113.10", "1080", false},
		{"addr set + boundary port 1", "203.0.113.10", "1", false},
		{"addr set + boundary port 65535", "203.0.113.10", "65535", false},
		{"addr empty + any port ignored", "", "abc", false},
		{"addr empty + empty port", "", "", false},
		{"addr set + empty port", "203.0.113.10", "", true},
		{"addr set + non-numeric port", "203.0.113.10", "abc", true},
		{"addr set + port zero", "203.0.113.10", "0", true},
		{"addr set + port too high", "203.0.113.10", "65536", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VK_TOKEN", "valid_token_for_test_xxxxxxxxxxxx")
			t.Setenv("OLCRTC_KEY", "deadbeef0123456789abcdef0123456789abcdef0123456789abcdef01234567")
			t.Setenv("SOCKS_PROXY_ADDR", tc.addr)
			t.Setenv("SOCKS_PROXY_PORT", tc.port)

			_, err := config.Load()
			if tc.wantErr && err == nil {
				t.Errorf("expected error for addr=%q port=%q, got nil", tc.addr, tc.port)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for addr=%q port=%q: %v", tc.addr, tc.port, err)
			}
			if tc.wantErr && err != nil && !errors.Is(err, config.ErrInvalidSocksPort) {
				t.Errorf("expected ErrInvalidSocksPort, got: %v", err)
			}
		})
	}
}
