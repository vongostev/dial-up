/*
[2026-07-09] :: 🗑️ :: Removed /wb /tm command test cases
*/

package tests

import (
	"testing"

	"dial-up/internal/provider"
)

func TestParseCommands(t *testing.T) {
	tests := []struct {
		input string
		kind  string
		room  string
		ok    bool
	}{
		{"/s", "", "", false},
		{"/n", "", "", false},
		{"/r", "", "", false},
		{"unknown", "", "", false},
	}

	for _, tt := range tests {
		p, ok := provider.Parse(tt.input)
		if ok != tt.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok {
			if p.Kind != tt.kind {
				t.Errorf("Parse(%q) kind = %q, want %q", tt.input, p.Kind, tt.kind)
			}
			if p.RoomID != tt.room {
				t.Errorf("Parse(%q) room = %q, want %q", tt.input, p.RoomID, tt.room)
			}
		}
	}
}

func TestParseURLs(t *testing.T) {
	tests := []struct {
		input string
		kind  string
		room  string
		ok    bool
	}{
		{"https://stream.wb.ru/room/019f33d5-c73d-7a09-ba85-b874bd1fceab", "wbstream", "019f33d5-c73d-7a09-ba85-b874bd1fceab", true},
		{"https://stream.wb.ru/room/abc-123", "wbstream", "abc-123", true},
		{"wbstream://abcdef1234567890abcdef1234567890ab", "wbstream", "abcdef1234567890abcdef1234567890ab", true},
		{"https://telemost.360.yandex.ru/j/1234567890", "telemost", "1234567890", true},
		{"https://stream.wb.ru/room/abc-1   ", "wbstream", "abc-1", true},
		{"https://stream.wb.ru/room/", "", "", false},
		{"wbstream://", "", "", false},
		{"https://telemost.360.yandex.ru/j/", "", "", false},
		{"http://stream.wb.ru/room/abc", "", "", false},
		{"https://telemost.360.yandex.ru/j/abc", "", "", false},
		{"https://stream.wb.ru/room/XYZ", "", "", false},
	}

	for _, tt := range tests {
		// input is lowercased before parsing (mirrors Python)
		p, ok := provider.Parse(tt.input)
		if ok != tt.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok {
			if p.Kind != tt.kind {
				t.Errorf("Parse(%q) kind = %q, want %q", tt.input, p.Kind, tt.kind)
			}
			if p.RoomID != tt.room {
				t.Errorf("Parse(%q) room = %q, want %q", tt.input, p.RoomID, tt.room)
			}
		}
	}
}

func TestParseCaseInsensitive(t *testing.T) {
	// Lowercase input ensures hex chars are lowercase
	_, ok := provider.Parse("https://stream.wb.ru/room/abcdef-123")
	if !ok {
		t.Error("Parse should match lowercase hex")
	}
}

func TestParseNoMatch(t *testing.T) {
	_, ok := provider.Parse("/s")
	if ok {
		t.Error("Parse should not match /s")
	}
}
