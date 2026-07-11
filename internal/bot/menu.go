/*
[2026-07-07] :: 🚀 :: Added modeButtons (Proxy/Direct) on second row for client; buildMenuKeyboard now accepts isClient
[2026-07-02] :: 🚀 :: Initial menu package
*/

package bot

import (
	"strings"

	"github.com/SevereCloud/vksdk/v3/object"
)

type buttonSpec struct {
	Label  string
	Action string
	Color  string
}

var menuButtons = []buttonSpec{
	{Label: "📊 Status", Action: "status", Color: "secondary"},
	{Label: "⏹ Stop", Action: "stop", Color: "negative"},
	{Label: "🔁 Restart", Action: "restart", Color: "positive"},
}

var modeButtons = []buttonSpec{
	{Label: "🌐 Proxy", Action: "mode-proxy", Color: "positive"},
	{Label: "🔗 Direct", Action: "mode-direct", Color: "secondary"},
}

func buildMenuKeyboard(isClient bool) *object.MessagesKeyboard {
	kb := object.NewMessagesKeyboard(object.BaseBoolInt(false))
	kb.AddRow()
	for _, spec := range menuButtons {
		kb.AddTextButton(spec.Label, struct{ Action string }{spec.Action}, spec.Color)
	}
	if isClient {
		kb.AddRow()
		for _, spec := range modeButtons {
			kb.AddTextButton(spec.Label, struct{ Action string }{spec.Action}, spec.Color)
		}
	}
	return kb
}

// - action: Token string (status|stop|restart|mode-proxy|mode-direct)
// - ok: true if match found

func resolveAction(text string) (string, bool) {
	t := strings.ToLower(text)
	for _, spec := range menuButtons {
		if strings.ToLower(spec.Label) == t {
			return spec.Action, true
		}
	}
	for _, spec := range modeButtons {
		if strings.ToLower(spec.Label) == t {
			return spec.Action, true
		}
	}
	return "", false
}
