#!/bin/sh
# =============================================================================
# Install dial-up on OpenWrt from GitHub
# Project: https://github.com/vongostev/OlcRTC-OpenWRT-VK-Bot
# Usage: sh -c "$(wget -qO- https://raw.githubusercontent.com/vongostev/OlcRTC-OpenWRT-VK-Bot/main/install.sh)"
# =============================================================================

set -e

REPO_RAW="https://raw.githubusercontent.com/vongostev/OlcRTC-OpenWRT-VK-Bot/main"
BOT_BINARY_URL="${REPO_RAW}/bin/dial-up-linux-arm64"
OLCRTC_BINARY_URL="${REPO_RAW}/bin/olcrtc-linux-arm64"
INIT_URL="${REPO_RAW}/deploy/openwrt/init.d/dial-up"
SINGBOX_CONFIG_URL="${REPO_RAW}/deploy/openwrt/sing-box-config.json"
WHITELIST_URL="${REPO_RAW}/deploy/openwrt/whitelist.json"
TPROXY_SETUP_URL="${REPO_RAW}/deploy/openwrt/setup-singbox-tproxy.sh"
ENV_SAMPLE_URL="${REPO_RAW}/deploy/openwrt/dial-up.env.sample"

BOT_BIN="/usr/bin/dial-up"
OLCRTC_BIN="/etc/olcrtc-linux-arm64"
INITD="/etc/init.d/dial-up"
ENV_FILE="/etc/dial-up.env"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!!]${NC} $*"; }
error() { echo -e "${RED}[ERR]${NC} $*"; exit 1; }
log() { printf '[..] %s\n' "$*"; }

echo ""
echo "╔══════════════════════════════════════╗"
echo "║   Install dial-up!                   ║"
echo "╚══════════════════════════════════════╝"
echo ""

# ── Pre-flight checks ─────────────────────────────────────
command -v wget >/dev/null 2>&1 || error "wget not found"
command -v uci  >/dev/null 2>&1 || error "uci not found (not OpenWrt?)"
command -v openssl >/dev/null 2>&1 || warn "openssl not found (will not generate key)"

# ── Detect architecture ────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    aarch64) info "Architecture: $ARCH" ;;
    armv7l)  warn "Architecture: $ARCH (armv7 not tested, binary may not work)" ;;
    *)       warn "Architecture: $ARCH (not aarch64, binary may not work)" ;;
esac

# ── Download bot binary ────────────────────────────────────
log "Downloading dial-up binary..."
wget -q -O "$BOT_BIN" "$BOT_BINARY_URL" || error "Failed to download bot binary"
chmod 755 "$BOT_BIN"
info "Bot binary: $BOT_BIN"

# ── Download olcrtc binary ─────────────────────────────────
log "Downloading olcrtc binary..."
wget -q -O "$OLCRTC_BIN" "$OLCRTC_BINARY_URL" || error "Failed to download olcrtc binary"
chmod 755 "$OLCRTC_BIN"
info "olcrtc binary: $OLCRTC_BIN"

# ── Download init script ───────────────────────────────────
log "Downloading init script..."
wget -q -O "$INITD" "$INIT_URL" || error "Failed to download init script"
chmod 755 "$INITD"
"$INITD" enable 2>/dev/null || warn "init.d enable failed"
info "Init script: $INITD (enabled)"

# ── Install required packages ─────────────────────────────
log "Installing required packages..."
PKGS=""
command -v sing-box >/dev/null 2>&1 || command -v sing-box-tiny >/dev/null 2>&1 || PKGS="$PKGS sing-box-tiny"
command -v setcap >/dev/null 2>&1 || PKGS="$PKGS libcap-bin"
opkg list-installed 2>/dev/null | grep -q kmod-nft-tproxy || PKGS="$PKGS kmod-nft-tproxy"
opkg list-installed 2>/dev/null | grep -q kmod-nf-tproxy || PKGS="$PKGS kmod-nf-tproxy"
if [ -n "$PKGS" ]; then
    log "Installing missing packages:$PKGS"
    opkg update >/dev/null 2>&1 || warn "opkg update failed"
    opkg install --force-space $PKGS >/dev/null 2>&1 || error "Failed to install packages:$PKGS"
    info "Packages installed successfully"
else
    info "All required packages are present"
fi

# ── Download sing-box config ──────────────────────────────
log "Downloading sing-box config..."
mkdir -p /etc/sing-box
if [ -f /etc/sing-box/config.json ]; then
    warn "sing-box config exists, backing up to /etc/sing-box/config.json.bak"
    cp /etc/sing-box/config.json /etc/sing-box/config.json.bak
fi
wget -q -O /etc/sing-box/config.json "$SINGBOX_CONFIG_URL" || error "Failed to download sing-box config"
info "sing-box config: /etc/sing-box/config.json"

log "Downloading whitelist..."
wget -q -O /etc/sing-box/whitelist.json "$WHITELIST_URL" || error "Failed to download whitelist"
info "whitelist: /etc/sing-box/whitelist.json"

# ── Enable sing-box service ───────────────────────────────
/etc/init.d/sing-box enable 2>/dev/null || warn "sing-box enable failed"
/etc/init.d/sing-box start 2>/dev/null || warn "sing-box start failed (may need config)"
info "sing-box service enabled"

# ── Setup tproxy rules ────────────────────────────────────
log "Downloading and running tproxy setup..."
wget -q -O /tmp/setup-singbox-tproxy.sh "$TPROXY_SETUP_URL" || error "Failed to download tproxy setup"
sh /tmp/setup-singbox-tproxy.sh
rm -f /tmp/setup-singbox-tproxy.sh
info "tproxy rules installed"

# ── Install LuCI app ─────────────────────────────────────
log "Installing LuCI olcRTC app..."
LUCI_BASE="${REPO_RAW}/luci-app-olcrtc"

mkdir -p /usr/libexec/rpcd
wget -q -O /usr/libexec/rpcd/olcrtc-bot "${LUCI_BASE}/root/usr/libexec/rpcd/olcrtc-bot" || warn "Failed to download rpcd backend"
chmod 755 /usr/libexec/rpcd/olcrtc-bot

mkdir -p /usr/share/rpcd/acl.d
wget -q -O /usr/share/rpcd/acl.d/luci-app-olcrtc.json "${LUCI_BASE}/root/usr/share/rpcd/acl.d/luci-app-olcrtc.json" || warn "Failed to download ACL"

mkdir -p /usr/share/luci/menu.d
wget -q -O /usr/share/luci/menu.d/luci-app-olcrtc.json "${LUCI_BASE}/root/usr/share/luci/menu.d/luci-app-olcrtc.json" || warn "Failed to download menu"

mkdir -p /www/luci-static/resources/view/olcrtc
for f in statusbar control data network network_client network_server logs; do
    wget -q -O "/www/luci-static/resources/view/olcrtc/$f.js" \
        "${LUCI_BASE}/htdocs/luci-static/resources/view/olcrtc/$f.js" || warn "Failed to download $f.js"
done

/etc/init.d/rpcd restart 2>/dev/null || warn "rpcd restart failed"
info "LuCI olcRTC app installed"

# ── Interactive configuration ──────────────────────────────
echo ""
echo "── Configuration ──────────────────────────"
echo ""

printf "VK_TOKEN (required): "
read VK_TOKEN_INPUT
while [ -z "$VK_TOKEN_INPUT" ]; do
    printf "${RED}[ERR]${NC} VK_TOKEN cannot be empty: "
    read VK_TOKEN_INPUT
done

printf "OLCRTC_KEY (press Enter to auto-generate): "
read OLCRTC_KEY_INPUT
if [ -z "$OLCRTC_KEY_INPUT" ]; then
    if command -v openssl >/dev/null 2>&1; then
        OLCRTC_KEY_INPUT=$(openssl rand -hex 32)
        info "OLCRTC_KEY auto-generated"
    else
        warn "openssl not found — enter OLCRTC_KEY manually above"
        printf "OLCRTC_KEY (required, no openssl): "
        read OLCRTC_KEY_INPUT
        while [ -z "$OLCRTC_KEY_INPUT" ]; do
            printf "${RED}[ERR]${NC} OLCRTC_KEY cannot be empty: "
            read OLCRTC_KEY_INPUT
        done
    fi
fi

printf "IS_CLIENT? [Y/n] (default: Y): "
read IS_CLIENT_INPUT
case "$IS_CLIENT_INPUT" in
    n|N|no|false) IS_CLIENT_VAL="false" ;;
    *)            IS_CLIENT_VAL="true" ;;
esac

printf "ALLOWED_USER_IDS (comma-separated, optional): "
read ALLOWED_USER_IDS_INPUT

printf "SOCKS_PROXY_ADDR (optional, e.g. 10.0.0.1): "
read SOCKS_PROXY_ADDR_INPUT

printf "SOCKS_PROXY_PORT (optional, default: 1080): "
read SOCKS_PROXY_PORT_INPUT

printf "SOCKS_PROXY_USER (optional): "
read SOCKS_PROXY_USER_INPUT

printf "SOCKS_PROXY_PASS (optional): "
read SOCKS_PROXY_PASS_INPUT

# ── Write env file ────────────────────────────────────────
OVERWRITE="y"
if [ -f "$ENV_FILE" ]; then
    printf "Config exists. Overwrite? [y/N] (default: N): "
    read OVERWRITE
fi

case "$OVERWRITE" in
    y|Y|yes)
        cat > "$ENV_FILE" << EOF
VK_TOKEN=$VK_TOKEN_INPUT
OLCRTC_KEY=$OLCRTC_KEY_INPUT
IS_CLIENT=$IS_CLIENT_VAL
DEBUG=false
OLCRTC_EXE=/etc/olcrtc-linux-arm64
DATA_DIR=data
LAST_PROVIDER_FILE=last_provider.json
ALLOWED_USER_IDS=$ALLOWED_USER_IDS_INPUT
SLEEP_ON_ERROR=5
STATUS_PORT=9091
SOCKS_PROXY_ADDR=$SOCKS_PROXY_ADDR_INPUT
SOCKS_PROXY_PORT=$SOCKS_PROXY_PORT_INPUT
SOCKS_PROXY_USER=$SOCKS_PROXY_USER_INPUT
SOCKS_PROXY_PASS=$SOCKS_PROXY_PASS_INPUT
EOF
        info "Env file created: $ENV_FILE"
        ;;
    *)
        info "Env file kept: $ENV_FILE"
        ;;
esac

# ── Start service ─────────────────────────────────────────
warn "Starting service..."
"$INITD" start 2>/dev/null || warn "Start failed (configure VK_TOKEN first)"

# ══════════════════════════════════════════════════════════════
# VERIFICATION
# ══════════════════════════════════════════════════════════════
echo ""
echo "── Verification ──────────────────────────"
echo ""

FAIL=0

check() {
    local desc="$1" cmd="$2"
    if eval "$cmd" >/dev/null 2>&1; then
        info "$desc"
    else
        printf "${RED}[ERR]${NC} %s\n" "$desc"
        FAIL=1
    fi
}

check "Bot binary exists and executable"  "[ -x $BOT_BIN ]"
check "olcrtc binary exists and executable" "[ -x $OLCRTC_BIN ]"
check "Init script exists"                "[ -f $INITD ]"
check "Init script enabled"           "[ -L /etc/rc.d/S99dial-up ] || [ -L /etc/rc.d/S*dial-up ]"
check "sing-box config exists"        "[ -f /etc/sing-box/config.json ]"
check "tproxy nft snippet exists"     "[ -f /etc/sing-box/tproxy.nft ]"
check "fw4 include registered"        "uci -q get firewall.singbox_tproxy.path >/dev/null"
check "tproxy nft chain active"       "nft list chain inet fw4 singbox_tproxy >/dev/null 2>&1"
check "LuCI olcRTC menu exists"       "[ -f /usr/share/luci/menu.d/luci-app-olcrtc.json ]"
check "rpcd backend exists"           "[ -x /usr/libexec/rpcd/olcrtc-bot ]"
check "rpcd ACL exists"               "[ -f /usr/share/rpcd/acl.d/luci-app-olcrtc.json ]"
check "LuCI control.js exists"        "[ -f /www/luci-static/resources/view/olcrtc/control.js ]"
check "LuCI statusbar.js exists"      "[ -f /www/luci-static/resources/view/olcrtc/statusbar.js ]"

OVERLAY_USAGE=$(df -h /overlay | awk 'NR==2{print $3" / "$2" ("$5" used)"}')
info "Overlay flash: $OVERLAY_USAGE"

echo ""
if [ "$FAIL" = "0" ]; then
    printf "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}\n"
    printf "${GREEN}║  Installation complete!                                  ║${NC}\n"
    printf "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}\n"
else
    printf "${RED}Some checks failed!${NC}\n"
fi
echo ""