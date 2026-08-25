#!/bin/sh
# =============================================================================
# setup-amnezia-awg.sh — provision AmneziaWG (AWG) on OpenWrt for dial-up!
#
# What it does:
#   1) installs amneziawg packages if missing (kmod-amneziawg, amneziawg-tools)
#   2) parses an AmneziaWG peer config (path/URL/paste) into UCI
#      (network.<iface> proto=amneziawg + peer section amneziawg_<iface>)
#   3) UCI policy routing: fwmark 0x2 -> table 102 with default dev <iface>
#      (mirror of the tproxy fwmark 0x1 -> table 100 pattern) so ONLY traffic
#      marked by sing-box's "awg" outbound (routing_mark: 2) uses the tunnel
#   4) adds <iface> to the lan firewall zone
#   5) writes AWG_* / DEFAULT_MODE / PROBE_URL into /etc/dial-up.env
#   6) updates /etc/sing-box/config.json to the version with the awg outbound
#      (bind_interface rewritten to <iface>), backs up the old one
#   7) restarts network (brings up the interface), sing-box and dial-up
#
# Usage:
#   setup-amnezia-awg.sh [path/to/peer.conf]        # file in AmneziaWG format
#   AWG_CONF_URL=... setup-amnezia-awg.sh           # download config first
#   INTERACTIVE=1 setup-amnezia-awg.sh              # paste config in prompt
#   setup-amnezia-awg.sh --skip-config              # iface already configured
#
# Overrides:
#   IFACE=awg0            interface name (default awg0)
#   DEFAULT_MODE=awg      standing route: awg | direct (default awg)
#   SINGBOX_CONFIG_URL=   sing-box config source (default GitHub raw main)
#   NO_SINGBOX=1          skip sing-box config update
#   NO_RESTART=1          do not restart services
# =============================================================================
set -u

IFACE="${IFACE:-awg0}"
DEFAULT_MODE="${DEFAULT_MODE:-awg}"
AWG_HANDSHAKE_MAX="${AWG_HANDSHAKE_MAX:-180}"
ENV_FILE="/etc/dial-up.env"
SINGBOX_DIR="/etc/sing-box"
CONF="${1:-}"
MODE="${1:-}"
[ "${MODE}" = "--skip-config" ] && CONF=""

REPO_RAW="https://raw.githubusercontent.com/vongostev/dial-up/main"
SINGBOX_CONFIG_URL="${SINGBOX_CONFIG_URL:-${REPO_RAW}/deploy/openwrt/sing-box-config.json}"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info() { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[!!]${NC} $*"; }
error() { echo -e "${RED}[ERR]${NC} $*"; exit 1; }
log()  { printf '[..] %s\n' "$*"; }

command -v uci >/dev/null 2>&1 || error "uci not found (not OpenWrt?)"

# ── 1) Packages ───────────────────────────────────────────────────────────
install_packages() {
    local missing=""
    opkg list-installed 2>/dev/null | grep -q '^kmod-amneziawg' || missing="$missing kmod-amneziawg"
    command -v awg >/dev/null 2>&1 || missing="$missing amneziawg-tools"
    opkg list-installed 2>/dev/null | grep -q '^luci-proto-amneziawg' || missing="$missing luci-proto-amneziawg"
    if [ -n "$missing" ]; then
        log "Installing missing packages:$missing"
        opkg update >/dev/null 2>&1 || warn "opkg update failed"
        opkg install --force-space $missing >/dev/null 2>&1 || error "Failed to install packages:$missing"
        info "Packages installed"
    else
        info "All amneziawg packages present"
    fi
}

# ── 2) Peer config → UCI ──────────────────────────────────────────────────
# Parse an AmneziaWG (wg-quick style) config: [Interface]/[Peer] key = value.
# Maps: PrivateKey→private_key, Address→addresses, MTU→mtu, PublicKey→public_key,
# PresharedKey→preshared_key, AllowedIPs→allowed_ips, Endpoint→endpoint_host/port,
# PersistentKeepalive→persistent_keepalive, Jc/Jmin/Jmax→awg_jc/jmin/jmax,
# S1..S4→awg_s1..s4, H1..H4→awg_h1..h4, I1..I5→awg_i1..i5.
conf_to_uci() {
    local conf="$1" section="" key="" val=""
    local private_key="" addresses="" mtu="" psk=""
    local public_key="" endpoint=""

    while IFS= read -r line || [ -n "$line" ]; do
        line=$(printf '%s' "$line" | sed 's/\r$//')
        case "$line" in
            ''|\#*) continue ;;
            '['*']')
                section=$(echo "$line" | tr -d '[]' | tr 'A-Z' 'a-z')
                continue
                ;;
        esac
        key=$(echo "$line" | cut -d= -f1 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        val=$(echo "$line" | cut -d= -f2- | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        [ -z "$key" ] && continue

        if [ "$section" = "interface" ]; then
            case "$key" in
                PrivateKey) private_key="$val" ;;
                Address)    for a in $(echo "$val" | tr ',' ' '); do addresses="$addresses $a"; done ;;
                MTU)        mtu="$val" ;;
                Jc|Jmin|Jmax|S1|S2|S3|S4|H1|H2|H3|H4|I1|I2|I3|I4|I5)
                    uci -q set network.${IFACE}.awg_$(echo "$key" | tr 'A-Z' 'a-z')="$val" ;;
            esac
        elif [ "$section" = "peer" ]; then
            case "$key" in
                PublicKey)         public_key="$val" ;;
                PresharedKey)      psk="$val" ;;
                Endpoint)          endpoint="$val" ;;
                AllowedIPs)       for a in $(echo "$val" | tr ',' ' '); do uci -q add_list network.${IFACE}peer.allowed_ips="$a"; done ;;
                PersistentKeepalive|PersistentKeepAlive) uci -q set network.${IFACE}peer.persistent_keepalive="$val" ;;
                Jc|Jmin|Jmax|S1|S2|S3|S4|H1|H2|H3|H4|I1|I2|I3|I4|I5)
                    uci -q set network.${IFACE}.awg_$(echo "$key" | tr 'A-Z' 'a-z')="$val" ;;
            esac
        fi
    done < "$conf"

    [ -n "$private_key" ] || error "config: missing [Interface] PrivateKey"
    [ -n "$public_key" ] || error "config: missing [Peer] PublicKey"

    uci -q set network.${IFACE}.private_key="$private_key"
    [ -n "$mtu" ] && uci -q set network.${IFACE}.mtu="$mtu"
    for a in $addresses; do
        uci -q add_list network.${IFACE}.addresses="$a"
    done
    uci -q set network.${IFACE}peer.public_key="$public_key"
    [ -n "$psk" ] && uci -q set network.${IFACE}peer.preshared_key="$psk"

    # Endpoint host:port (IPv6 [..]:port supported)
    if [ -n "$endpoint" ]; then
        case "$endpoint" in
            \[*\]:*)
                uci -q set network.${IFACE}peer.endpoint_host="$(echo "$endpoint" | sed 's/^\[\(.*\)\]:.*$/\1/')"
                uci -q set network.${IFACE}peer.endpoint_port="$(echo "$endpoint" | sed 's/^\[.*\]:\(.*\)$/\1/')"
                ;;
            *:*)
                uci -q set network.${IFACE}peer.endpoint_host="$(echo "$endpoint" | cut -d: -f1)"
                uci -q set network.${IFACE}peer.endpoint_port="$(echo "$endpoint" | cut -d: -f2-)"
                ;;
            *)
                uci -q set network.${IFACE}peer.endpoint_host="$endpoint"
                uci -q set network.${IFACE}peer.endpoint_port="51820"
                ;;
        esac
    fi
}

obtain_conf() {
    local tmp="/tmp/awg-peer.conf.$$"
    if [ -n "$CONF" ] && [ -f "$CONF" ]; then
        cp "$CONF" "$tmp"
    elif [ -n "${AWG_CONF_URL:-}" ]; then
        log "Downloading peer config from $AWG_CONF_URL..."
        wget -q -O "$tmp" "$AWG_CONF_URL" || error "Failed to download $AWG_CONF_URL"
    elif [ "${INTERACTIVE:-0}" = "1" ]; then
        echo "Paste the AmneziaWG peer config, then press Ctrl-D:"
        cat > "$tmp"
    else
        rm -f "$tmp"
        error "No peer config: pass a file, set AWG_CONF_URL, or use INTERACTIVE=1"
    fi
    echo "$tmp"
}

configure_uci() {
    log "Configuring UCI interface $IFACE (proto=amneziawg)..."
    uci -q delete network.${IFACE}
    uci -q delete network.${IFACE}peer
    uci set network.${IFACE}=interface
    uci set network.${IFACE}.proto='amneziawg'
    uci set network.${IFACE}peer=amneziawg_${IFACE}

    local conf_tmp
    conf_tmp=$(obtain_conf)
    trap 'rm -f "$conf_tmp"' EXIT
    conf_to_uci "$conf_tmp"
    rm -f "$conf_tmp"
    trap - EXIT

    # route_allowed_ips stays 0 (default) — no kernel routes from AllowedIPs.
    uci commit network
    info "UCI interface + peer written"
}

# ── 3) Policy routing (fwmark 0x2 -> table 102) ──────────────────────────
configure_routing() {
    log "UCI policy routing (fwmark 0x2 -> table 102)..."
    uci -q delete network.awg_rule
    uci set network.awg_rule=rule
    uci set network.awg_rule.mark='2'
    uci set network.awg_rule.lookup='102'

    uci -q delete network.awg_default
    uci set network.awg_default=route
    uci set network.awg_default.interface="$IFACE"
    uci set network.awg_default.target='0.0.0.0'
    uci set network.awg_default.netmask='0.0.0.0'
    uci set network.awg_default.table='102'
    uci commit network
    info "Policy routing committed"
}

# ── 4) Firewall: add interface to lan zone ────────────────────────────────
configure_firewall() {
    local zone
    zone=$(uci show firewall 2>/dev/null | grep -E "\.name=['\"]lan['\"]" | cut -d. -f1-2 | head -1)
    if [ -z "$zone" ]; then
        warn "lan zone not found — firewall zone for $IFACE not configured"
        return 0
    fi
    # Idempotent: add to the zone only if not already listed (uci add_list does not dedupe).
    if uci -q get "$zone.network" 2>/dev/null | grep -qw "$IFACE"; then
        info "$IFACE already in $zone (lan), skipping"
    else
        uci -q add_list "$zone.network=$IFACE"
        info "$IFACE added to $zone (lan)"
    fi
    uci commit firewall
}

# ── 5) dial-up env ────────────────────────────────────────────────────────
update_env() {
    [ -f "$ENV_FILE" ] || touch "$ENV_FILE"
    set_env_val() {
        local k="$1" v="$2"
        if grep -q "^$k=" "$ENV_FILE" 2>/dev/null; then
            sed -i "s|^$k=.*|$k=$v|" "$ENV_FILE"
        else
            echo "$k=$v" >> "$ENV_FILE"
        fi
    }
    set_env_val "AWG_IFACE" "$IFACE"
    set_env_val "AWG_HANDSHAKE_MAX" "$AWG_HANDSHAKE_MAX"
    set_env_val "DEFAULT_MODE" "$DEFAULT_MODE"
    grep -q '^PROBE_URL=' "$ENV_FILE" 2>/dev/null || \
        set_env_val "PROBE_URL" "https://www.google.com/generate_204"
    info "dial-up env updated (AWG_IFACE=$IFACE DEFAULT_MODE=$DEFAULT_MODE)"
}

# ── 6) sing-box config with awg outbound ─────────────────────────────────
update_singbox() {
    [ "${NO_SINGBOX:-0}" = "1" ] && { warn "NO_SINGBOX=1, skipping"; return 0; }
    [ -d "$SINGBOX_DIR" ] || mkdir -p "$SINGBOX_DIR"
    local cfg="$SINGBOX_DIR/config.json"
    if [ -f "$cfg" ]; then
        log "Backing up $cfg"
        cp "$cfg" "${cfg}.bak-$(date +%Y%m%d-%H%M%S)"
        # Keep only the 3 newest backups so idempotent re-runs do not accumulate files.
        ls -1t "${cfg}".bak-* 2>/dev/null | tail -n +4 | xargs -r rm -f 2>/dev/null || true
    fi
    log "Downloading sing-box config with awg outbound..."
    wget -q -O "$cfg" "$SINGBOX_CONFIG_URL" || error "Failed to download sing-box config"
    # Rewrite bind_interface to the actual interface name (default awg0).
    sed -i "s/\"bind_interface\": \"awg0\"/\"bind_interface\": \"$IFACE\"/" "$cfg"
    if ! grep -q '"tag": "awg"' "$cfg"; then
        warn "awg outbound not found in downloaded config — check SINGBOX_CONFIG_URL"
    fi
    /etc/init.d/sing-box restart 2>/dev/null || warn "sing-box restart failed"
    info "sing-box config updated and restarted"
}

# ── 7) Bring up / restart ────────────────────────────────────────────────
restart_services() {
    [ "${NO_RESTART:-0}" = "1" ] && { warn "NO_RESTART=1, skipping restarts"; return 0; }
    log "Applying network config..."
    /etc/init.d/network reload 2>/dev/null || /etc/init.d/network restart 2>/dev/null || warn "network reload failed"
    ifup "$IFACE" 2>/dev/null || warn "ifup $IFACE failed (interface may need LuCI/WAN setup)"
    sleep 2
    /etc/init.d/dial-up restart 2>/dev/null || warn "dial-up restart failed"
}

# ── Verification ──────────────────────────────────────────────────────────
verify() {
    echo ""
    echo "── Verification ──────────────────────────"
    echo ""
    [ -n "$IFACE" ] && uci -q get network.${IFACE}.proto >/dev/null 2>&1 && \
        info "UCI: network.$IFACE proto=$(uci -q get network.${IFACE}.proto)" || \
        error "UCI: network.$IFACE missing"
    if ip link show dev "$IFACE" >/dev/null 2>&1; then
        info "Interface: $IFACE up"
        awg show "$IFACE" 2>/dev/null | head -5 || true
        hs=$(awg show "$IFACE" latest-handshakes 2>/dev/null | awk '{print $2}')
        if [ -n "$hs" ] && [ "$hs" != "0" ]; then
            info "Latest handshake: $(date -d @$hs '+%Y-%m-%d %H:%M:%S' 2>/dev/null || echo epoch $hs)"
        else
            warn "No handshake yet — check endpoint reachability and keys"
        fi
    else
        warn "Interface $IFACE not up — check UCI settings / LuCI network page"
    fi
    ip rule show 2>/dev/null | grep -q "fwmark 0x2" && info "ip rule: fwmark 0x2 -> table 102" || warn "ip rule fwmark 0x2 missing"
    ip route show table 102 2>/dev/null | grep -q "dev $IFACE" && info "route table 102: default dev $IFACE" || warn "route table 102 missing"
    echo ""
    info "Next: send a room link to the bot (or /m awg) — the awg route is now available."
    echo ""
}

# ══════════════════════════════════════════════════════════════════════════
# MAIN
# ══════════════════════════════════════════════════════════════════════════
echo ""
echo "╔══════════════════════════════════════╗"
echo "║   AmneziaWG setup for dial-up!       ║"
echo "║   IFACE:        ${IFACE}"
echo "║   DEFAULT_MODE: ${DEFAULT_MODE}"
echo "╚══════════════════════════════════════╝"
echo ""

[ "$(id -u)" = "0" ] || error "must run as root"

install_packages

if [ -n "$CONF" ]; then
    configure_uci
elif [ "${INTERACTIVE:-0}" = "1" ] || [ -n "${AWG_CONF_URL:-}" ]; then
    configure_uci
else
    warn "--skip-config: assuming the interface was configured elsewhere (LuCI/UCI)"
    [ -n "$IFACE" ] || IFACE=$(uci show network 2>/dev/null | grep -E "\.proto='amneziawg'" | head -1 | cut -d. -f2)
    [ -n "$IFACE" ] || error "no amneziawg interface found; pass a peer config or set AWG_IFACE"
    uci -q get network.${IFACE}.proto >/dev/null 2>&1 || \
        error "network.$IFACE is not amneziawg — configure it first"
fi

configure_routing
configure_firewall
update_env
update_singbox
restart_services
verify

