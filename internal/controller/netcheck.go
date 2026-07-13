/*
[2026-07-10] :: 🚀 :: Added PortOpen(addr, timeout) TCP liveness check for the tproxy guardian's proxy-mode race-window detection
[2026-07-08] :: 🚀 :: Initial PingDNS implementation via TCP connect to 9.9.9.9:53
*/

package controller

import (
	"context"
	"net"
	"strings"
	"time"
)

// PingDNS measures internet connectivity via TCP connect to 9.9.9.9:53 (Quad9 DNS).
// Returns RTT as a human-readable string on success, or an error description on failure.
func PingDNS(ctx context.Context) string {
	const target = "9.9.9.9:53"
	const timeout = 2 * time.Second

	start := time.Now()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	rtt := time.Since(start)

	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
			return "timeout"
		case strings.Contains(msg, "connection refused"):
			return "connection refused"
		default:
			return "unavailable"
		}
	}
	conn.Close()

	return rtt.Round(time.Millisecond).String()
}

// - addr: TCP address (host:port) to probe
// - timeout: Maximum connect duration

// PortOpen reports whether a TCP connection to addr succeeds within the timeout.
// Used by the guardian to detect whether olcrtc's SOCKS5 listener is alive.
func PortOpen(addr string, timeout time.Duration) bool {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
