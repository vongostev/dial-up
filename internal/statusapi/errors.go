/*
[2026-07-09] :: 🚀 :: Initial statusapi sentinel errors
*/

// Package statusapi exposes the bot's authoritative state over a loopback HTTP endpoint.
package statusapi

import "errors"

var (
	// ErrStatusBind is returned when the status endpoint cannot bind its loopback address.
	ErrStatusBind = errors.New("status endpoint bind failed")
)
