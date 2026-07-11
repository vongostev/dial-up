/*
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
}

// RenderConfig renders the olcrtc YAML config by substituting template variables.
func RenderConfig(p RenderParams) string {
	tpl := srvTemplate
	if p.IsClient {
		tpl = cncTemplate
	}
	r := strings.NewReplacer(
		"$provider", p.Provider,
		"$room_id", p.RoomID,
		"$olcrtc_key", p.OlcrtcKey,
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
