/*
[2026-08-24] :: 🛡️ :: Liveness: timeout 5s/7s -> 45s and failures 3 -> 4 in both templates — vp8channel's control plane legitimately goes silent 25-30s under load (KCP batching + SFU publisher renegotiation, olcrtc issue #95 / commit 384268b), so the tight timeouts caused false-positive 'reason=liveness' session kills ~1.5 min after connect
[2026-08-24] :: 🐛 :: Drop `data: data` from both templates: olcrtc >= 46b82c6 hard-fails when a configured data dir lacks names/surnames dictionaries (dial-up never ships them), while an empty `data:` uses the embedded dictionaries — the old olcrtc required `data:` but silently fell back to embedded when files were missing
[2026-07-31] :: 🚀 :: Added NetDNS to RenderParams + $net_dns substitution (configurable server egress DNS)
[2026-07-10] :: 🚀 :: Added RenderParams + buildSocksProxyBlock for server-mode SOCKS5 egress; rewritten RenderConfig(RenderParams)
[2026-07-02] :: 🚀 :: Initial render package
*/

package controller

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed templates/cnc.yaml
var cncTemplate string

//go:embed templates/srv.yaml
var srvTemplate string

// defaultNetDNS is used when RenderParams.NetDNS is empty, so the server
// config always carries a valid resolver (defaults to the local sing-box DNS).
const defaultNetDNS = "127.0.0.1:53"

// RenderParams holds all inputs needed to render an olcrtc YAML config.
type RenderParams struct {
	IsClient       bool
	Provider       string
	RoomID         string
	OlcrtcKey      string
	SocksProxyAddr string
	SocksProxyPort string
	SocksProxyUser string
	SocksProxyPass string
	NetDNS         string
}

// RenderConfig renders the olcrtc YAML config by substituting template variables.
func RenderConfig(p RenderParams) string {
	tpl := srvTemplate
	if p.IsClient {
		tpl = cncTemplate
	}
	dns := p.NetDNS
	if dns == "" {
		dns = defaultNetDNS
	}
	r := strings.NewReplacer(
		"$provider", p.Provider,
		"$room_id", p.RoomID,
		"$olcrtc_key", p.OlcrtcKey,
		"$net_dns", dns,
		"$socks_proxy\n", buildSocksProxyBlock(p),
	)
	return r.Replace(tpl)
}

// buildSocksProxyBlock returns the upstream SOCKS5 egress block or "" when disabled.
func buildSocksProxyBlock(p RenderParams) string {
	if p.IsClient || p.SocksProxyAddr == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("socks:\n")
	fmt.Fprintf(&b, "  proxy_addr: %q\n", p.SocksProxyAddr)
	fmt.Fprintf(&b, "  proxy_port: %s\n", p.SocksProxyPort)
	if p.SocksProxyUser != "" {
		fmt.Fprintf(&b, "  proxy_user: %q\n", p.SocksProxyUser)
	}
	if p.SocksProxyPass != "" {
		fmt.Fprintf(&b, "  proxy_pass: %q\n", p.SocksProxyPass)
	}
	return b.String()
}
