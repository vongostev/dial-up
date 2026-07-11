/*
[2026-07-07] :: 🔌 :: Stop label now asserts ClearProvider (was SetProvider) since /n stop fully forgets the provider
[2026-07-02] :: 🔌 :: Updated expectations for symmetric role-emoji prefix (📡/📺) and StatusText()
*/

package tests

import (
	"context"
	"strings"
	"testing"

	"dial-up/internal/bot"
	"dial-up/internal/config"
	"dial-up/internal/domain/logger"
)

func TestMenuSendsKeyboard(t *testing.T) {
	sender := &fakeSender{}
	ctrl := &fakeController{}
	cfg := &config.Config{IsClient: false}
	l := logger.New(true)
	b := bot.New(nil, ctrl, cfg, l)
	b.SetSender(sender)

	ctx := context.Background()
	b.HandleMessage(ctx, 123, "/menu")

	if sender.keyboard == nil {
		t.Error("expected non-nil keyboard for /menu command")
	}
	if sender.text == "" {
		t.Error("expected response text for /menu command")
	}
}

func TestButtonLabelsDispatch(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		isClient bool
		check    func(*testing.T, *fakeSender, *fakeController)
	}{
		{
			name:     "Status label",
			label:    "📊 Status",
			isClient: false,
			check: func(t *testing.T, s *fakeSender, _ *fakeController) {
				t.Helper()

				if s.text != "📡 Сервер fake-status" {
					t.Errorf("status response = %q, want %q", s.text, "📡 Сервер fake-status")
				}
			},
		},
		{
			name:     "Stop label",
			label:    "⏹ Stop",
			isClient: false,
			check: func(t *testing.T, s *fakeSender, c *fakeController) {
				t.Helper()

				if !c.clearProviderCalled {
					t.Error("expected ClearProvider to be called for stop")
				}
				if s.text != "📡 Сервер Ok" {
					t.Errorf("stop response = %q, want %q", s.text, "📡 Сервер Ok")
				}
			},
		},
		{
			name:     "Restart label",
			label:    "🔁 Restart",
			isClient: false,
			check: func(t *testing.T, s *fakeSender, c *fakeController) {
				t.Helper()

				if !c.restartCalled {
					t.Error("expected Restart to be called")
				}
				if s.text != "📡 Сервер Ok" {
					t.Errorf("restart response = %q, want %q", s.text, "📡 Сервер Ok")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSender{}
			ctrl := &fakeController{}
			cfg := &config.Config{IsClient: tt.isClient}
			l := logger.New(true)
			b := bot.New(nil, ctrl, cfg, l)
			b.SetSender(sender)

			ctx := context.Background()
			b.HandleMessage(ctx, 123, tt.label)

			tt.check(t, sender, ctrl)
		})
	}
}

func TestFallbackCommandsStillWork(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantText string
	}{
		{"/s", "/s", "📡 Сервер fake-status"},
		{"/n", "/n", "📡 Сервер Ok"},
		{"/r", "/r", "📡 Сервер Ok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSender{}
			ctrl := &fakeController{}
			cfg := &config.Config{IsClient: false}
			l := logger.New(true)
			b := bot.New(nil, ctrl, cfg, l)
			b.SetSender(sender)

			ctx := context.Background()
			b.HandleMessage(ctx, 123, tt.cmd)

			if sender.text != tt.wantText {
				t.Errorf("got %q, want %q", sender.text, tt.wantText)
			}
		})
	}
}

func TestMenuClientPrefix(t *testing.T) {
	sender := &fakeSender{}
	ctrl := &fakeController{}
	cfg := &config.Config{IsClient: true}
	l := logger.New(true)
	b := bot.New(nil, ctrl, cfg, l)
	b.SetSender(sender)

	ctx := context.Background()
	b.HandleMessage(ctx, 123, "/menu")

	if !strings.HasPrefix(sender.text, "📺 Клиент ") {
		t.Errorf("expected client prefix 📺 Клиент , got %q", sender.text)
	}
	if sender.keyboard == nil {
		t.Error("expected non-nil keyboard for /menu on client")
	}
}
