package tests

import (
	"strings"
	"testing"

	"dial-up/internal/controller"
	"dial-up/internal/provider"
)

// canonicalClient is the expected output for a client render.
const canonicalClient = `mode: cnc
auth:
  provider: wbstream
room:
  id: "abc-123"
crypto:
  key: "deadbeef0123456789"
net:
  transport: vp8channel
  dns: "77.88.8.8:53"
socks:
  host: "127.0.0.1"
  port: 1080
data: data
debug: false
liveness:
  interval: 10s
  timeout: 7s
  failures: 3
`

const canonicalServer = `mode: srv
auth:
  provider: telemost
room:
  id: "room-42"
crypto:
  key: "cafebabe9876543210"
net:
  transport: vp8channel
  dns: "9.9.9.9:53"
data: data
debug: false
liveness:
  interval: 10s
  timeout: 5s
  failures: 3
`

func TestRenderClient(t *testing.T) {
	got := controller.RenderConfig(controller.RenderParams{
		IsClient:  true,
		Provider:  provider.ProviderWbStream,
		RoomID:    "abc-123",
		OlcrtcKey: "deadbeef0123456789",
	})
	if got != canonicalClient {
		t.Errorf("Client render mismatch:\ngot:\n%s\n\nwant:\n%s", got, canonicalClient)
	}
}

func TestRenderServer(t *testing.T) {
	got := controller.RenderConfig(controller.RenderParams{
		IsClient:  false,
		Provider:  provider.ProviderTelemost,
		RoomID:    "room-42",
		OlcrtcKey: "cafebabe9876543210",
	})
	if got != canonicalServer {
		t.Errorf("Server render mismatch:\ngot:\n%s\n\nwant:\n%s", strings.TrimSpace(got), canonicalServer)
	}
}

func TestRenderNoDollarLeftovers(t *testing.T) {
	got := controller.RenderConfig(controller.RenderParams{
		IsClient:  true,
		Provider:  provider.ProviderWbStream,
		RoomID:    "abc",
		OlcrtcKey: "key",
	})
	if strings.Contains(got, "$") {
		t.Errorf("Render left unsubstituted $ in:\n%s", got)
	}
}

// TestRenderServerWithSocks asserts the server socks block with full auth is rendered correctly.
func TestRenderServerWithSocks(t *testing.T) {
	got := controller.RenderConfig(controller.RenderParams{
		IsClient:       false,
		Provider:       provider.ProviderTelemost,
		RoomID:         "room-42",
		OlcrtcKey:      "cafebabe9876543210",
		SocksProxyAddr: "203.0.113.10",
		SocksProxyPort: "1080",
		SocksProxyUser: "myuser",
		SocksProxyPass: "secretpass",
	})

	wantBlock := `socks:
  proxy_addr: "203.0.113.10"
  proxy_port: 1080
  proxy_user: "myuser"
  proxy_pass: "secretpass"
data: data`
	if !strings.Contains(got, wantBlock) {
		t.Errorf("expected socks block not found in:\n%s", got)
	}
	if strings.Contains(got, "$") {
		t.Errorf("Render left unsubstituted $ in:\n%s", got)
	}
}

// TestRenderServerSocksNoAuth asserts the server socks block without auth omits user/pass.
func TestRenderServerSocksNoAuth(t *testing.T) {
	got := controller.RenderConfig(controller.RenderParams{
		IsClient:       false,
		Provider:       provider.ProviderTelemost,
		RoomID:         "room-42",
		OlcrtcKey:      "cafebabe9876543210",
		SocksProxyAddr: "203.0.113.10",
		SocksProxyPort: "1080",
	})

	wantBlock := `socks:
  proxy_addr: "203.0.113.10"
  proxy_port: 1080
data: data`
	if !strings.Contains(got, wantBlock) {
		t.Errorf("expected no-auth socks block not found in:\n%s", got)
	}
	if strings.Contains(got, "proxy_user") || strings.Contains(got, "proxy_pass") {
		t.Errorf("no-auth block must not contain proxy_user/proxy_pass:\n%s", got)
	}
}

// TestRenderServerNoSocks asserts no socks block when the address is empty.
func TestRenderServerNoSocks(t *testing.T) {
	got := controller.RenderConfig(controller.RenderParams{
		IsClient:  false,
		Provider:  provider.ProviderTelemost,
		RoomID:    "room-42",
		OlcrtcKey: "cafebabe9876543210",
	})
	if strings.Contains(got, "socks") {
		t.Errorf("no socks block expected when addr is empty:\n%s", got)
	}
	if strings.Contains(got, "$socks_proxy") {
		t.Errorf("placeholder must be fully removed:\n%s", got)
	}
	// No blank line where the placeholder was: dns line directly followed by data line.
	if !strings.Contains(got, `  dns: "9.9.9.9:53"
data: data`) {
		t.Errorf("expected dns line directly followed by data line (no blank line):\n%s", got)
	}
}

// TestRenderClientIgnoresSocks asserts client render ignores SOCKS_PROXY_* fields.
func TestRenderClientIgnoresSocks(t *testing.T) {
	got := controller.RenderConfig(controller.RenderParams{
		IsClient:       true,
		Provider:       provider.ProviderWbStream,
		RoomID:         "abc-123",
		OlcrtcKey:      "deadbeef0123456789",
		SocksProxyAddr: "203.0.113.10",
		SocksProxyPort: "1080",
		SocksProxyUser: "myuser",
		SocksProxyPass: "secretpass",
	})
	if got != canonicalClient {
		t.Errorf("Client render should ignore SOCKS_PROXY_*:\ngot:\n%s\n\nwant:\n%s", got, canonicalClient)
	}
}
