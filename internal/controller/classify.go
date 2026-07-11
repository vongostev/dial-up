/*
[2026-07-07] :: 🛡️ :: Narrowed definitive tokens to anchored phrases ("status 403", "auth failed") to avoid false-positive removal on benign lines (e.g. "port 4030", "authority")
[2026-07-07] :: 🚀 :: Initial classifier: definitive (403/forbidden/auth/room/provider-kind) vs transient (default) — drives immediate last_provider.json removal on definitive tunnel rejection
*/

package controller

import "strings"

// FailureClass classifies a captured olcrtc output line as transient or definitive.
type FailureClass int

// FailureClass values. ClassTransient = retry with backoff + maxFailures safety net (keep file);
// ClassDefinitive = provider permanently rejected, remove last_provider.json now.
const (
	ClassTransient FailureClass = iota
	ClassDefinitive
)

// ClassifyOutput returns ClassDefinitive for permanent tunnel rejections (status 403 /
// auth failed / forbidden / room / provider-kind) and ClassTransient for anything else
// (429/net/dns/timeout/unknown).
func ClassifyOutput(line string) FailureClass {
	s := strings.ToLower(strings.TrimSpace(line))
	// Previously a 403/forbidden crash ran the full ~23 min maxFailures cascade before
	// last_provider.json was removed, hammering an upstream that had already permanently rejected
	// the provider. Definitive classification collapses that to a single crash.
	if strings.Contains(s, "status 403") ||
		strings.Contains(s, "forbidden") ||
		strings.Contains(s, "auth failed") ||
		strings.Contains(s, "cannot create room") ||
		strings.Contains(s, "room not found") ||
		strings.Contains(s, "room invalid") ||
		strings.Contains(s, "unknown provider") {
		return ClassDefinitive
	}
	return ClassTransient
}
