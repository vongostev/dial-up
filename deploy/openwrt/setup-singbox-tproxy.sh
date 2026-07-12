#!/bin/sh
# =============================================================================
# OpenWrt transparent-proxy setup for sing-box tproxy (port 2080).
#
# Design:
#   1) UCI policy routing  — fwmark 0x1 -> table 100 (persists in /etc/config/network)
#   2) nft tproxy snippet  — own chain with prerouting/mangle hook
#   3) fw4 include (UCI)   — snippet auto-applied on every fw4 reload / reboot
#
# sing-box uses a "selector" outbound (route-select) defaulting to "direct".
# When olcrtc tunnel is up the bot switches selector to "proxy" via Clash API;
# when olcrtc is down the selector stays/returns to "direct".
# Therefore tproxy rules can stay permanently — traffic safety is handled
# by the selector, not by presence/absence of nft rules.
# =============================================================================
set -u

FWMARK=0x1
TABLE=100

GREEN='\033[32m'
RED='\033[31m'
YELLOW='\033[33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { printf "${GREEN}[OK]${NC} %s\n" "$*"; }
warn()  { printf "${YELLOW}[!!]${NC} %s\n" "$*"; }
error() { printf "${RED}[ERR]${NC} %s\n" "$*"; exit 1; }
log() { printf '[..] %s\n' "$*"; }

# ---------------------------------------------------------------------------
# 1) UCI policy routing: fwmark 0x1 -> table 100 (local route for tproxy)
# ---------------------------------------------------------------------------
log "UCI network policy routing (fwmark ${FWMARK} -> table ${TABLE})"

uci -q delete network.tproxy_rule
uci set network.tproxy_rule=rule
uci set network.tproxy_rule.mark="${FWMARK#0x}"
uci set network.tproxy_rule.lookup="$TABLE"

uci -q delete network.tproxy_local
uci set network.tproxy_local=route
uci set network.tproxy_local.interface='loopback'
uci set network.tproxy_local.target='0.0.0.0'
uci set network.tproxy_local.netmask='0.0.0.0'
uci set network.tproxy_local.table="$TABLE"
uci set network.tproxy_local.type='local'

uci commit network
info "UCI network committed (rule + local route in table ${TABLE})"

# ---------------------------------------------------------------------------
# 2) nft tproxy snippet — own chain inside fw4 table, auto-loaded by fw4 include.
#    Wrapped in 'table inet fw4 { ... }' so fw4 merges it into the existing table.
# ---------------------------------------------------------------------------
log "Writing nft snippet /etc/sing-box/tproxy.nft"
mkdir -p /etc/sing-box
cat > /etc/sing-box/tproxy.nft <<'EOF'
chain singbox_tproxy {
  type filter hook prerouting priority mangle; policy accept;
  iifname "br-lan" ip daddr { 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 } counter accept
  iifname "br-lan" meta nfproto ipv4 udp dport { 67, 68 } counter accept
  iifname "br-lan" meta nfproto ipv4 meta l4proto { tcp, udp } counter meta mark set 0x1 tproxy ip to 127.0.0.1:2080 accept
}
EOF
info "nft snippet written"

# ---------------------------------------------------------------------------
# 3) Register as fw4 include so rules survive fw4 reload / reboot.
#    No position/chain needed — we declare our own hook (prerouting mangle).
# ---------------------------------------------------------------------------
log "Registering fw4 firewall include"
uci -q delete firewall.singbox_tproxy
uci set firewall.singbox_tproxy=include
uci set firewall.singbox_tproxy.type='nftables'
uci set firewall.singbox_tproxy.path='/etc/sing-box/tproxy.nft'
uci commit firewall
info "firewall include registered (singbox_tproxy)"

log "Reloading fw4..."
if fw4 reload 2>/dev/null; then
  info "fw4 reloaded"
else
  error "fw4 reload failed"
fi

log "Applying UCI network rules..."
if /etc/init.d/network restart 2>/dev/null; then
  info "network rules applied (fwmark routing active)"
else
  warn "service network restart failed — run manually or reboot"
fi

log "Configuring sing-box UCI (enable + root for tproxy)..."
uci -q delete sing-box.main.enabled 2>/dev/null
uci set sing-box.main.enabled='1'
uci -q delete sing-box.main.user 2>/dev/null
uci set sing-box.main.user='root'
uci commit sing-box 2>/dev/null || warn "uci commit sing-box failed — maybe no uci config exists yet"
info "sing-box UCI: enabled=1, user=root"

# tproxy inbound requires CAP_NET_ADMIN to set the IP_TRANSPARENT socket option,
# otherwise sing-box fails with "listen tcp4 0.0.0.0:2080: operation not permitted".
# Granting capabilities on the binary is reliable regardless of the procd run user;
# needs the libcap-bin package (opkg install libcap-bin).
SB_BIN="$(command -v sing-box 2>/dev/null || command -v sing-box-tiny 2>/dev/null)"
if [ -n "${SB_BIN:-}" ] && command -v setcap >/dev/null 2>&1; then
	if setcap 'cap_net_admin,cap_net_raw,cap_net_bind_service=+ep' "$SB_BIN" 2>/dev/null; then
		info "capabilities granted on ${SB_BIN}"
	else
		warn "setcap failed on ${SB_BIN} (fs without xattr?) — relying on user=root"
	fi
else
	warn "setcap unavailable (opkg install libcap-bin) — relying on user=root"
fi

log "Restarting sing-box (re-applies UCI user/root)..."
if /etc/init.d/sing-box restart 2>/dev/null; then
  info "sing-box restarted"
else
  warn "sing-box restart failed — check config or start manually"
fi

echo ""
echo "── Sing-box verification ──────────────────────────"
echo ""

if uci -q get firewall.singbox_tproxy.path >/dev/null 2>&1; then
	info "firewall.singbox_tproxy.type = $(uci -q get firewall.singbox_tproxy.type)"
	info "firewall.singbox_tproxy.path = $(uci -q get firewall.singbox_tproxy.path)"
else
	error 'firewall.singbox_tproxy UCI section MISSING\n'
fi

if uci -q get network.tproxy_rule.mark >/dev/null 2>&1; then
	info "network.tproxy_rule  mark=$(uci -q get network.tproxy_rule.mark) lookup=$(uci -q get network.tproxy_rule.lookup)"
else
	error 'network.tproxy_rule UCI section MISSING\n'
fi
if uci -q get network.tproxy_local.table >/dev/null 2>&1; then
	info "network.tproxy_local table=$(uci -q get network.tproxy_local.table) type=$(uci -q get network.tproxy_local.type)"
else
	error 'network.tproxy_local UCI section MISSING\n'
fi

if [ -f /etc/sing-box/tproxy.nft ]; then
	LINES=$(wc -l < /etc/sing-box/tproxy.nft)
	info "/etc/sing-box/tproxy.nft exists (${LINES} lines)"
else
	error '/etc/sing-box/tproxy.nft NOT FOUND\n'
fi

RULE=$(ip rule show 2>/dev/null | grep "fwmark ${FWMARK}")
if [ -n "$RULE" ]; then
	info "ip rule: $RULE"
else
	warn "ip rule for fwmark ${FWMARK} MISSING (run 'service network restart' or reboot)"
fi
ROUTE=$(ip route show table ${TABLE} 2>/dev/null | grep local)
if [ -n "$ROUTE" ]; then
	info "ip route table ${TABLE}: $ROUTE"
else
	warn "ip route table ${TABLE} local route MISSING (run 'service network restart' or reboot)"
fi

if nft list chain inet fw4 singbox_tproxy >/dev/null 2>&1; then
	NRULES=$(nft list chain inet fw4 singbox_tproxy 2>/dev/null | grep -c 'tproxy\|accept')
	info "singbox_tproxy chain ACTIVE (${NRULES} rules)"
else
	error 'singbox_tproxy chain ABSENT (applied on next fw4 reload or reboot)\n'
fi

if netstat -ltn 2>/dev/null | grep -q ":2080 "; then
	info "sing-box listening on :2080"
else
	warn "sing-box NOT listening on :2080 (check config, binary, or start manually)"
fi