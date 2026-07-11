/*
[2026-07-10] :: 🚀 :: Added PortOpen tests: open TCP listener → true, closed port → false
[2026-07-08] :: 🚀 :: Initial PingDNS test suite
*/

package tests

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"dial-up/internal/controller"
)

func TestPingDNSReturnsNonEmpty(t *testing.T) {
	result := controller.PingDNS()
	if result == "" {
		t.Fatal("PingDNS() returned empty string, expected RTT or error text")
	}
	t.Logf("PingDNS() = %q", result)
}

func TestPingDNSResultFormat(t *testing.T) {
	result := controller.PingDNS()
	validPatterns := []string{"ms", "µs", "0s", "timeout", "connection refused", "unavailable"}

	found := false
	for _, p := range validPatterns {
		if strings.Contains(result, p) {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("PingDNS() = %q, expected to contain one of %v", result, validPatterns)
	}
}

func TestPortOpenReturnsTrueForListeningPort(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if !controller.PortOpen(ln.Addr().String(), 1*time.Second) {
		t.Errorf("PortOpen(%q) = false, want true", ln.Addr().String())
	}
	t.Logf("PortOpen(%q) = true", ln.Addr().String())
}

func TestPortOpenReturnsFalseForClosedPort(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	if controller.PortOpen(addr, 1*time.Second) {
		t.Errorf("PortOpen(%q) = true, want false", addr)
	}
	t.Logf("PortOpen(%q) = false", addr)
}
