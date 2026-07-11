/*
[2026-07-09] :: 🚀 :: Added json:"kind"/json:"room_id" struct tags so controller.Status serializes Provider for the local status endpoint
[2026-07-09] :: 🗑️ :: Removed /wb /tm command prefix parsing; only URL-based parsing remains
[2026-07-02] :: 🚀 :: Initial provider package
*/

// Package provider handles parsing of commands and URLs into provider tuples.
package provider

import (
	"regexp"
)

const ProviderWbStream = "wbstream"
const ProviderTelemost = "telemost"

// Provider represents a media provider kind and room identifier.
type Provider struct {
	Kind   string `json:"kind"`
	RoomID string `json:"room_id"`
}

var (
	// Exact regexes matching Python proxy_bot — input pre-lowercased, NO TrimSpace, \s*$ kept.
	re1 = regexp.MustCompile(`^https://stream\.wb\.ru/room/([0-9abcdef-]+)\s*$`)
	re2 = regexp.MustCompile(`^wbstream://([0-9abcdef-]+)\s*$`)
	re3 = regexp.MustCompile(`^https://telemost\.360\.yandex\.ru/j/([0-9]+)\s*$`)
	re4 = regexp.MustCompile(`^https://telemost\.yandex\.ru/j/([0-9]+)\s*$`)
)

// Parse parses input text into a provider kind and room ID.
func Parse(text string) (Provider, bool) {
	if m := re1.FindStringSubmatch(text); m != nil {
		return Provider{Kind: ProviderWbStream, RoomID: m[1]}, true
	}
	if m := re2.FindStringSubmatch(text); m != nil {
		return Provider{Kind: ProviderWbStream, RoomID: m[1]}, true
	}
	if m := re3.FindStringSubmatch(text); m != nil {
		return Provider{Kind: ProviderTelemost, RoomID: m[1]}, true
	}
	if m := re4.FindStringSubmatch(text); m != nil {
		return Provider{Kind: ProviderTelemost, RoomID: m[1]}, true
	}
	return Provider{}, false
}
