/*
[2026-07-07] :: 🚀 :: Initial classifyOutput table test covering definitive (403/forbidden/auth/room/provider) vs transient (429/timeout/net/dns/unknown) inputs
*/

package tests

import (
	"testing"

	"dial-up/internal/controller"
)

func TestClassifyOutput(t *testing.T) {
	tests := []struct {
		name string
		line string
		want controller.FailureClass
	}{
		// Definitive — provider permanently rejected by the tunnel.
		{"status 403", "carrier auth failed: status 403 guests cannot create rooms", controller.ClassDefinitive},
		{"forbidden", "ERROR forbidden", controller.ClassDefinitive},
		{"auth failed", "carrier auth failed: invalid signature", controller.ClassDefinitive},
		{"cannot create rooms", "guests cannot create rooms", controller.ClassDefinitive},
		{"room not found", "room not found", controller.ClassDefinitive},
		{"room invalid", "ERROR room invalid", controller.ClassDefinitive},
		{"unknown provider", "unknown provider kind", controller.ClassDefinitive},

		// Transient — retryable; backoff + maxFailures safety net apply.
		{"429", "429 too many requests", controller.ClassTransient},
		{"timeout", "dial tcp: i/o timeout", controller.ClassTransient},
		{"connection refused", "connection refused", controller.ClassTransient},
		{"no such host", "dial tcp: lookup x: no such host", controller.ClassTransient},
		{"empty", "", controller.ClassTransient},
		{"unknown gibberish", "something completely unrelated", controller.ClassTransient},
		// False-positive guards: bare "403"/"auth" substrings must NOT trigger definitive removal.
		{"port 4030 not 403", "listening on port 4030", controller.ClassTransient},
		{"authority not auth-failed", "certificate authority established", controller.ClassTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := controller.ClassifyOutput(tt.line)
			if got != tt.want {
				t.Errorf("classifyOutput(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
