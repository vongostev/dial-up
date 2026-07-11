/*
[2026-07-07] :: 🚀 :: Added mode-proxy/mode-direct button dispatch via handleSetRouteMode; buildMenuKeyboard now receives isClient for conditional second row
[2026-07-07] :: 🚀 :: Added ClearProvider() to Controller interface; handleStop ("/n" / Stop button) now routes through ClearProvider so stop means forget — the running tunnel is killed AND last_provider.json is deleted (no auto-reconnect after reboot)
[2026-07-07] :: 🚀 :: Added SetRoute to Controller interface; /m proxy|direct command handler
[2026-07-06] :: 🐛 :: Guarded messages.send against vksdk typed-nil errors (non-nil interface wrapping nil pointer) that rendered as "%!w(<nil>)" and masked the real outcome; genuine API errors now propagate with their real message
[2026-07-02] :: 🎨 :: Symmetric role-emoji prefix (📡 Сервер / 📺 Клиент) replacing client-only C: ; status via StatusText()
[2026-07-02] :: 🏗️ :: Extracted status/stop/restart into Bot methods to eliminate dispatch drift
[2026-07-02] :: 🚀 :: Added button menu: Sender.Send now accepts *object.MessagesKeyboard; handle dispatches /menu and label actions
[2026-07-02] :: 🚀 :: Initial bot package
*/

// Package bot implements a VK long-poll bot with command dispatch and auto-reconnect.
package bot

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"dial-up/internal/config"
	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
	"dial-up/internal/provider"
	"dial-up/internal/singbox"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/api/params"
	longpoll "github.com/SevereCloud/vksdk/v3/longpoll-user"
	wrapper "github.com/SevereCloud/vksdk/v3/longpoll-user/v3"
	"github.com/SevereCloud/vksdk/v3/object"
)

// Sender abstracts VK message sending for testability.
type Sender interface {
	Send(peerID int, text string, keyboard *object.MessagesKeyboard) error
}

// Controller abstracts the subprocess controller API for bot dispatch.
type Controller interface {
	SetProvider(p *provider.Provider, save bool)
	ClearProvider()
	Restart()
	Status() controller.Status
	StatusText() string
	SetRoute(mode string) error
}

const serverPrefix = "📡 Сервер "

const clientPrefix = "📺 Клиент "

// Bot is a VK user long-poll bot with command dispatch and auto-reconnect.
type Bot struct {
	vk         *api.VK
	controller Controller
	cfg        *config.Config
	l          logger.Logger
	sender     Sender
	allowedIDs map[int]bool
}

// New creates a Bot with the given VK client, controller, config, and logger.
func New(vk *api.VK, ctrl Controller, cfg *config.Config, l logger.Logger) *Bot {
	b := &Bot{
		vk:         vk,
		controller: ctrl,
		cfg:        cfg,
		l:          l.With(logger.Function("Bot")),
		sender:     &vkSender{vk: vk},
		allowedIDs: make(map[int]bool),
	}
	if cfg.AllowedUserIDs != "" {
		for _, s := range strings.Split(cfg.AllowedUserIDs, ",") {
			s = strings.TrimSpace(s)
			if id, err := strconv.Atoi(s); err == nil {
				b.allowedIDs[id] = true
			}
		}
	}
	return b
}

// Run starts the long-poll loop with exponential backoff reconnection.
func (b *Bot) Run(ctx context.Context) error {
	cl := b.l.With(logger.Function("Bot.Run"))

	lp, err := longpoll.NewLongPoll(b.vk, longpoll.ReceiveAttachments|longpoll.ExtendedEvents)
	if err != nil {
		return fmt.Errorf("failed to create longpoll: %w", err)
	}
	lp.Wait = 5

	w := wrapper.NewWrapper(lp)
	w.OnNewMessage(b.handle)

	backoff := 1 * time.Second
	const maxBackoff = 60 * time.Second

	for {
		cl.Info("bot", "Starting long-poll handler", logger.Status("ATTEMPT"), logger.Importance(5))

		runErr := lp.Run()

		select {
		case <-ctx.Done():
			cl.Info("bot", "Context cancelled, shutting down", logger.Status("OK"), logger.Importance(5))
			lp.Shutdown()
			return nil
		default:
		}

		if runErr != nil {
			cl.Warn("bot", "Long-poll error", logger.Block("Reconnect"), logger.Status("FAIL"), logger.Importance(6), logger.Error(runErr))
		} else {
			cl.Warn("bot", "Long-poll stopped, reconnecting", logger.Block("Reconnect"), logger.Status("SKIP"), logger.Importance(5))
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			lp.Shutdown()
			return nil
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (b *Bot) handle(m wrapper.NewMessage) {
	cl := b.l.With(logger.Function("Bot.handle"))

	if m.Flags.Has(wrapper.Outbox) {
		return
	}

	if len(b.allowedIDs) > 0 && !b.allowedIDs[m.PeerID] {
		cl.Warn("bot", "Denied non-allowed user", logger.Block("Allowlist"), logger.Status("SKIP"), logger.Importance(5), logger.Int("peerID", m.PeerID))
		return
	}

	text := strings.ToLower(m.Text)

	var responseText string
	var kb *object.MessagesKeyboard

	switch text {
	case "/menu", "/start":
		responseText = "Для управления используйте кнопки Status / Stop / Restart. Для подключения к комнате отправьте URL."
		kb = buildMenuKeyboard(b.cfg.IsClient)
		cl.Info("bot", "Menu keyboard attached", logger.Block("CommandDispatch"), logger.Status("OK"), logger.Importance(5))
	case "/s":
		responseText = b.handleStatus()
	case "/n":
		responseText = b.handleStop()
	case "/r":
		responseText = b.handleRestart()
	case "/m":
		if b.cfg.IsClient {
			responseText = b.handleSetRoute(text)
		}
	default:
		if action, ok := resolveAction(text); ok {
			cl.Info("bot", "Button label dispatch", logger.Block("CommandDispatch"), logger.Status("OK"), logger.Importance(5), logger.String("action", action))
			switch action {
			case "status":
				responseText = b.handleStatus()
			case "stop":
				responseText = b.handleStop()
			case "restart":
				responseText = b.handleRestart()
			case "mode-proxy":
				if b.cfg.IsClient {
					responseText = b.handleSetRouteMode(singbox.ModeProxy)
				}
			case "mode-direct":
				if b.cfg.IsClient {
					responseText = b.handleSetRouteMode(singbox.ModeDirect)
				}
			}
		} else if p, ok := provider.Parse(text); ok {
			b.controller.SetProvider(&p, true)
			responseText = "Ok"
		} else if !b.cfg.IsClient {
			responseText = "Unknown command"
		}
	}

	if responseText != "" {
		rolePrefix := serverPrefix
		if b.cfg.IsClient {
			rolePrefix = clientPrefix
		}
		responseText = rolePrefix + responseText
		if err := b.sender.Send(m.PeerID, responseText, kb); err != nil {
			cl.Error("bot", "Send failed", logger.Block("Send"), logger.Status("FAIL"), logger.Importance(7), logger.Error(err))
		} else {
			cl.Info("bot", "Sent response", logger.Block("Send"), logger.Status("OK"), logger.Importance(5), logger.String("text", responseText))
		}
	}
}

func (b *Bot) handleStatus() string {
	if b.cfg.IsClient {
		time.Sleep(time.Second)
	}

	return b.controller.StatusText()
}

func (b *Bot) handleStop() string {
	b.controller.ClearProvider()
	return "Ok"
}

func (b *Bot) handleRestart() string {
	b.controller.Restart()
	return "Ok"
}

func (b *Bot) handleSetRoute(text string) string {
	b.l.Info("bot", "")
	parts := strings.Fields(text)
	if len(parts) != 2 {
		return "Usage: /m proxy | /m direct"
	}
	mode := strings.ToLower(parts[1])
	if mode != singbox.ModeProxy && mode != singbox.ModeDirect {
		return "Некорректный режим. Используй proxy или direct"
	}
	if err := b.controller.SetRoute(mode); err != nil {
		return "Ошибка: " + err.Error()
	}
	return "Маршрут установлен: " + mode
}

// handleSetRouteMode delegates a pre-validated mode to controller.SetRoute.
func (b *Bot) handleSetRouteMode(mode string) string {
	if err := b.controller.SetRoute(mode); err != nil {
		return "Ошибка: " + err.Error()
	}
	return "Маршрут установлен: " + mode
}

type vkSender struct {
	vk *api.VK
}

func (s *vkSender) Send(peerID int, text string, keyboard *object.MessagesKeyboard) error {
	b := params.NewMessagesSendBuilder()
	b.PeerID(peerID)
	b.Message(text)
	b.RandomID(0)
	if keyboard != nil {
		b.Keyboard(keyboard)
	}
	_, err := s.vk.MessagesSend(b.Params)
	if err == nil {
		return nil
	}
	// vksdk returns a typed-nil error (non-nil interface wrapping a nil pointer) for
	// messages.send, which renders as "%!w(<nil>)" via %w and masks the real situation. Treat a
	// typed-nil as success; any genuine API error still propagates with its real message.
	if isNilError(err) {
		return nil
	}
	return fmt.Errorf("cannot send message: %w", err)
}

// SetSender overrides the message sender for testing.
func (b *Bot) SetSender(s Sender) {
	b.sender = s
}

// HandleMessage simulates an incoming message for testing.
func (b *Bot) HandleMessage(_ context.Context, peerID int, text string) {
	m := wrapper.NewMessage{
		Flags: 0,
	}
	m.PeerID = peerID
	m.Text = text
	b.handle(m)
}

// isNilError reports whether err is a typed-nil (non-nil interface around a nil pointer), which
// vksdk returns from messages.send when there is no real error.
func isNilError(err error) bool {
	v := reflect.ValueOf(err)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
