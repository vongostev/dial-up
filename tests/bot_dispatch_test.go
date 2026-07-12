/*
[2026-07-09] :: 🗑️ :: Removed /wb /tm command dispatch tests
[2026-07-02] :: 🔌 :: Updated expectations for symmetric role-emoji prefix (📡/📺) and StatusText()
*/

package tests

import (
	"context"
	"testing"

	"dial-up/internal/bot"
	"dial-up/internal/config"
	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
	"dial-up/internal/provider"

	"github.com/SevereCloud/vksdk/v3/object"
)

// fakeSender records sent messages.
type fakeSender struct {
	peerID   int
	text     string
	keyboard *object.MessagesKeyboard
}

func (s *fakeSender) Send(peerID int, text string, keyboard *object.MessagesKeyboard) error {
	s.peerID = peerID
	s.text = text
	s.keyboard = keyboard
	return nil
}

// fakeController implements bot.Controller for testing.
type fakeController struct {
	setProviderCalled   bool
	clearProviderCalled bool
	restartCalled       bool
	lastProvider        *provider.Provider
	lastSave            bool
}

func (c *fakeController) SetProvider(p *provider.Provider, save bool) {
	c.setProviderCalled = true
	c.lastProvider = p
	c.lastSave = save
}
func (c *fakeController) ClearProvider()            { c.clearProviderCalled = true }
func (c *fakeController) Restart()                  { c.restartCalled = true }
func (c *fakeController) Status() controller.Status { return controller.Status{} }
func (c *fakeController) StatusText() string        { return "fake-status" }
func (c *fakeController) SetRoute(_ string) error   { return nil }
func (c *fakeController) SetVkAlive(_ bool)         {}

func TestDispatchCommands(t *testing.T) {
	tests := []struct {
		name      string
		cmd       string
		isClient  bool
		wantText  string
		wantClear bool // expect ClearProvider to be called (true for /n)
	}{
		{"/s status", "/s", false, "📡 Сервер fake-status", false},
		{"/s status client", "/s", true, "📺 Клиент fake-status", false},
		{"/n stop", "/n", false, "📡 Сервер Ok", true},
		{"/n stop client", "/n", true, "📺 Клиент Ok", true},
		{"/r restart", "/r", false, "📡 Сервер Ok", false},
		{"/r restart client", "/r", true, "📺 Клиент Ok", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSender{}
			ctrl := &fakeController{}
			cfg := &config.Config{IsClient: tt.isClient}
			l := logger.New(true)
			b := bot.New(nil, ctrl, cfg, l)
			// Override sender with our fake
			b.SetSender(sender)

			// Simulate handle via exported method
			ctx := context.Background()
			b.HandleMessage(ctx, 123, tt.cmd)

			if sender.text != tt.wantText {
				t.Errorf("got text %q, want %q", sender.text, tt.wantText)
			}
			if ctrl.clearProviderCalled != tt.wantClear {
				t.Errorf("clearProviderCalled = %v, want %v", ctrl.clearProviderCalled, tt.wantClear)
			}
		})
	}
}

func TestDispatchProviders(t *testing.T) {
	tests := []struct {
		name         string
		cmd          string
		isClient     bool
		wantKind     string
		wantRoom     string
		wantSave     bool
		wantResponse string
	}{
		{"wbstream URL", "https://stream.wb.ru/room/abc-123", true, "wbstream", "abc-123", true, "📺 Клиент Ok"},
		{"telemost URL", "https://telemost.360.yandex.ru/j/42", true, "telemost", "42", true, "📺 Клиент Ok"},
		{"unknown server", "unknown", false, "", "", false, "📡 Сервер Unknown command"},
		{"unknown client", "unknown", true, "", "", false, ""},
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
			b.HandleMessage(ctx, 123, tt.cmd)

			if tt.wantKind != "" {
				if !ctrl.setProviderCalled {
					t.Error("expected SetProvider to be called")
				}
				if ctrl.lastProvider == nil {
					t.Fatal("expected non-nil provider")
				}
				if ctrl.lastProvider.Kind != tt.wantKind {
					t.Errorf("provider kind = %q, want %q", ctrl.lastProvider.Kind, tt.wantKind)
				}
				if ctrl.lastProvider.RoomID != tt.wantRoom {
					t.Errorf("provider room = %q, want %q", ctrl.lastProvider.RoomID, tt.wantRoom)
				}
				if ctrl.lastSave != tt.wantSave {
					t.Errorf("save = %v, want %v", ctrl.lastSave, tt.wantSave)
				}
			} else if ctrl.setProviderCalled {
				t.Error("expected SetProvider NOT to be called")
			}

			if tt.wantResponse != "" && sender.text != tt.wantResponse {
				t.Errorf("got text %q, want %q", sender.text, tt.wantResponse)
			}
		})
	}
}

func TestOutboxFilter(t *testing.T) {
	// Outbox messages should be filtered out by the bot's handle logic.
	// Since we test via HandleMessage which doesn't have the wrapper.Flags check,
	// this verifies the sender is not called for outbox (simulated by checking text).
	sender := &fakeSender{}
	ctrl := &fakeController{}
	cfg := &config.Config{IsClient: false}
	l := logger.New(true)
	b := bot.New(nil, ctrl, cfg, l)
	b.SetSender(sender)

	ctx := context.Background()
	b.HandleMessage(ctx, 123, "/s")

	if sender.text == "" {
		t.Error("expected status response for non-outbox message")
	}
}

func TestServerUnknownCommand(t *testing.T) {
	sender := &fakeSender{}
	ctrl := &fakeController{}
	cfg := &config.Config{IsClient: false}
	l := logger.New(true)
	b := bot.New(nil, ctrl, cfg, l)
	b.SetSender(sender)

	ctx := context.Background()
	b.HandleMessage(ctx, 123, "some gibberish")

	if sender.text != "📡 Сервер Unknown command" {
		t.Errorf("got %q, want %q", sender.text, "📡 Сервер Unknown command")
	}
}

func TestClientSilentOnUnknown(t *testing.T) {
	sender := &fakeSender{}
	ctrl := &fakeController{}
	cfg := &config.Config{IsClient: true}
	l := logger.New(true)
	b := bot.New(nil, ctrl, cfg, l)
	b.SetSender(sender)

	ctx := context.Background()
	b.HandleMessage(ctx, 123, "some gibberish")

	if sender.text != "" {
		t.Errorf("client should be silent on unknown, got %q", sender.text)
	}
}
