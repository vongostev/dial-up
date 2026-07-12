#!/bin/sh
# =============================================================================
# Install / Upgrade dial-up on OpenWrt from GitHub
# Project: https://github.com/vongostev/OlcRTC-OpenWRT-VK-Bot
#
# Modes:
#   Fresh install (default)     : sh -c "$(wget -qO- .../install.sh)"
#   Server mode                 : INSTALL_TYPE=server sh ...install.sh
#   Non-interactive (headless)  : NONINTERACTIVE=1 VK_TOKEN=xxx OLCRTC_KEY=yyy sh ...install.sh
#   Upgrade (auto-detected)     : repeats install.sh on existing router
# =============================================================================

set -e

# ── Environment overrides ─────────────────────────────────
INSTALL_TYPE="${INSTALL_TYPE:-client}"   # client | server
NONINTERACTIVE="${NONINTERACTIVE:-0}"    # 1 = headless, 0 = interactive prompt

# Known env vars used in NONINTERACTIVE mode
VK_TOKEN="${VK_TOKEN:-}"
OLCRTC_KEY="${OLCRTC_KEY:-}"
ALLOWED_USER_IDS="${ALLOWED_USER_IDS:-}"
SOCKS_PROXY_ADDR="${SOCKS_PROXY_ADDR:-}"
SOCKS_PROXY_PORT="${SOCKS_PROXY_PORT:-1080}"
SOCKS_PROXY_USER="${SOCKS_PROXY_USER:-}"
SOCKS_PROXY_PASS="${SOCKS_PROXY_PASS:-}"

# ── URLs ──────────────────────────────────────────────────
REPO_RAW="https://raw.githubusercontent.com/vongostev/OlcRTC-OpenWRT-VK-Bot/main"
BOT_BINARY_URL="${REPO_RAW}/bin/dial-up-linux-arm64"
OLCRTC_BINARY_URL="${REPO_RAW}/bin/olcrtc-linux-arm64"
INIT_URL="${REPO_RAW}/deploy/openwrt/init.d/dial-up"
SINGBOX_CONFIG_URL="${REPO_RAW}/deploy/openwrt/sing-box-config.json"
WHITELIST_URL="${REPO_RAW}/deploy/openwrt/whitelist.json"
TPROXY_SETUP_URL="${REPO_RAW}/deploy/openwrt/setup-singbox-tproxy.sh"
ENV_SAMPLE_URL="${REPO_RAW}/deploy/openwrt/dial-up.env.sample"

# ── Paths ─────────────────────────────────────────────────
BOT_BIN="/usr/bin/dial-up"
OLCRTC_BIN="/etc/olcrtc-linux-arm64"
INITD="/etc/init.d/dial-up"
ENV_FILE="/etc/dial-up.env"
SINGBOX_DIR="/etc/sing-box"

# ── Colors ────────────────────────────────────────────────
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!!]${NC} $*"; }
error() { echo -e "${RED}[ERR]${NC} $*"; exit 1; }
log()   { printf '[..] %s\n' "$*"; }

# ── Helpers ───────────────────────────────────────────────

# Portable file hash (hex digest). Empty string if unavailable.
hash_file() {
    if command -v md5sum >/dev/null 2>&1; then
        md5sum "$1" 2>/dev/null | awk '{print $1}'
    elif command -v md5 >/dev/null 2>&1; then
        md5 -q "$1" 2>/dev/null
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -md5 "$1" 2>/dev/null | awk '{print $NF}'
    else
        echo ""
    fi
}

# Atomic binary download with optional skip-if-same-hash.
# Usage: download_binary <url> <dest_path> <description>
download_binary() {
    local url="$1" dest="$2" desc="$3"
    local tmpfile="/tmp/.$(basename "$dest").$$"

    log "Downloading $desc..."
    wget -q -O "$tmpfile" "$url" || error "Failed to download $desc"
    chmod 755 "$tmpfile"

    # Skip overwrite if destination already has identical hash
    if [ -f "$dest" ]; then
        local dest_hash tmp_hash
        dest_hash=$(hash_file "$dest") || true
        tmp_hash=$(hash_file "$tmpfile") || true
        if [ -n "$dest_hash" ] && [ -n "$tmp_hash" ] && [ "$dest_hash" = "$tmp_hash" ]; then
            info "$desc: $dest (unchanged, skipped)"
            rm -f "$tmpfile"
            return 0
        fi
    fi

    mv -f "$tmpfile" "$dest" || error "Failed to install $desc"
    rm -f "$tmpfile"
    info "$desc: $dest"
}

# Stop dial-up service and wait for process disappearance.
stop_service() {
    if [ -x "$INITD" ]; then
        log "Stopping service..."
        "$INITD" stop 2>/dev/null || true
        local i=0
        while [ "$i" -lt 10 ]; do
            if ! pgrep -f "$BOT_BIN" >/dev/null 2>&1; then
                break
            fi
            sleep 1
            i=$((i + 1))
        done
    fi
}

# Kill any running olcrtc binary and wait.
kill_olcrtc() {
    log "Killing any running olcrtc processes..."
    killall -q "$(basename "$OLCRTC_BIN")" 2>/dev/null || true
    local i=0
    while [ "$i" -lt 5 ]; do
        if ! pgrep -f "$OLCRTC_BIN" >/dev/null 2>&1; then
            break
        fi
        sleep 1
        i=$((i + 1))
        done
}

# Install required opkg packages (client only).
install_packages() {
    if [ "$INSTALL_TYPE" = "server" ]; then
        info "Server mode: skipping sing-box / tproxy packages"
        return 0
    fi

    log "Checking required packages..."
    local PKGS=""
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
}

# Install sing-box config and whitelist (client only).
install_singbox_config() {
    if [ "$INSTALL_TYPE" = "server" ]; then
        return 0
    fi

    log "Installing sing-box config..."
    mkdir -p "$SINGBOX_DIR"
    if [ -f "$SINGBOX_DIR/config.json" ]; then
        warn "sing-box config exists, backing up to config.json.bak"
        cp "$SINGBOX_DIR/config.json" "$SINGBOX_DIR/config.json.bak"
    fi
    wget -q -O "$SINGBOX_DIR/config.json" "$SINGBOX_CONFIG_URL" || error "Failed to download sing-box config"
    info "sing-box config: $SINGBOX_DIR/config.json"

    log "Downloading whitelist..."
    wget -q -O "$SINGBOX_DIR/whitelist.json" "$WHITELIST_URL" || error "Failed to download whitelist"
    info "whitelist: $SINGBOX_DIR/whitelist.json"

    /etc/init.d/sing-box enable 2>/dev/null || warn "sing-box enable failed"
    /etc/init.d/sing-box start 2>/dev/null || warn "sing-box start failed (may need config)"
    info "sing-box service enabled"
}

# Install TProxy rules (client only).
install_tproxy() {
    if [ "$INSTALL_TYPE" = "server" ]; then
        return 0
    fi

    log "Installing tproxy rules..."
    local tmp_script="/tmp/setup-singbox-tproxy.sh.$$"
    wget -q -O "$tmp_script" "$TPROXY_SETUP_URL" || error "Failed to download tproxy setup"
    sh "$tmp_script"
    rm -f "$tmp_script"
    info "tproxy rules installed"
}

# Install LuCI application files.
install_luci() {
    log "Installing LuCI olcRTC app..."
    local LUCI_BASE="${REPO_RAW}/luci-app-olcrtc"

    mkdir -p /usr/libexec/rpcd
    wget -q -O /usr/libexec/rpcd/olcrtc-bot "${LUCI_BASE}/root/usr/libexec/rpcd/olcrtc-bot" || warn "Failed to download rpcd backend"
    chmod 755 /usr/libexec/rpcd/olcrtc-bot

    mkdir -p /usr/share/rpcd/acl.d
    wget -q -O /usr/share/rpcd/acl.d/luci-app-olcrtc.json "${LUCI_BASE}/root/usr/share/rpcd/acl.d/luci-app-olcrtc.json" || warn "Failed to download ACL"

    mkdir -p /usr/share/luci/menu.d
    wget -q -O /usr/share/luci/menu.d/luci-app-olcrtc.json "${LUCI_BASE}/root/usr/share/luci/menu.d/luci-app-olcrtc.json" || warn "Failed to download menu"

    mkdir -p /www/luci-static/resources/view/olcrtc
    for f in statusbar bot tunnel network network_client network_server logs; do
        wget -q -O "/www/luci-static/resources/view/olcrtc/$f.js" \
            "${LUCI_BASE}/htdocs/luci-static/resources/view/olcrtc/$f.js" || warn "Failed to download $f.js"
    done

    /etc/init.d/rpcd restart 2>/dev/null || warn "rpcd restart failed"
    info "LuCI olcRTC app installed"
}

# Write /etc/dial-up.env. Interactive or headless depending on NONINTERACTIVE.
write_env() {
    log "Configuring environment file..."

    if [ "$NONINTERACTIVE" = "1" ]; then
        # Headless mode: require VK_TOKEN; generate OLCRTC_KEY if missing and openssl available
        if [ -z "$VK_TOKEN" ]; then
            error "NONINTERACTIVE=1 but VK_TOKEN is empty"
        fi
        if [ -z "$OLCRTC_KEY" ]; then
            if command -v openssl >/dev/null 2>&1; then
                OLCRTC_KEY=$(openssl rand -hex 32)
                info "OLCRTC_KEY auto-generated"
            else
                error "NONINTERACTIVE=1 but OLCRTC_KEY is empty and openssl not found"
            fi
        fi

        local IS_CLIENT_VAL="true"
        [ "$INSTALL_TYPE" = "server" ] && IS_CLIENT_VAL="false"

        cat > "$ENV_FILE" << ENV_EOF
VK_TOKEN=$VK_TOKEN
OLCRTC_KEY=$OLCRTC_KEY
IS_CLIENT=$IS_CLIENT_VAL
DEBUG=false
OLCRTC_EXE=/etc/olcrtc-linux-arm64
DATA_DIR=data
LAST_PROVIDER_FILE=last_provider.json
ALLOWED_USER_IDS=$ALLOWED_USER_IDS
SLEEP_ON_ERROR=5
STATUS_PORT=9091
SOCKS_PROXY_ADDR=$SOCKS_PROXY_ADDR
SOCKS_PROXY_PORT=$SOCKS_PROXY_PORT
SOCKS_PROXY_USER=$SOCKS_PROXY_USER
SOCKS_PROXY_PASS=$SOCKS_PROXY_PASS
ENV_EOF
        info "Env file created (non-interactive): $ENV_FILE"
        return 0
    fi

    # Interactive mode
    echo ""
    echo "── Configuration ──────────────────────────"
    echo ""

    local VK_TOKEN_INPUT=""
    printf "VK_TOKEN (required): "
    read VK_TOKEN_INPUT
    while [ -z "$VK_TOKEN_INPUT" ]; do
        printf "${RED}[ERR]${NC} VK_TOKEN cannot be empty: "
        read VK_TOKEN_INPUT
    done

    local OLCRTC_KEY_INPUT=""
    printf "OLCRTC_KEY (press Enter to auto-generate): "
    read OLCRTC_KEY_INPUT
    if [ -z "$OLCRTC_KEY_INPUT" ]; then
        if command -v openssl >/dev/null 2>&1; then
            OLCRTC_KEY_INPUT=$(openssl rand -hex 32)
            info "OLCRTC_KEY auto-generated"
        else
            warn "openssl not found — enter OLCRTC_KEY manually"
            printf "OLCRTC_KEY (required, no openssl): "
            read OLCRTC_KEY_INPUT
            while [ -z "$OLCRTC_KEY_INPUT" ]; do
                printf "${RED}[ERR]${NC} OLCRTC_KEY cannot be empty: "
                read OLCRTC_KEY_INPUT
            done
        fi
    fi

    local IS_CLIENT_VAL="true"
    if [ "$INSTALL_TYPE" = "server" ]; then
        IS_CLIENT_VAL="false"
        info "Server mode selected: IS_CLIENT=false"
    else
        printf "IS_CLIENT? [Y/n] (default: Y): "
        local IS_CLIENT_INPUT=""
        read IS_CLIENT_INPUT
        case "$IS_CLIENT_INPUT" in
            n|N|no|false) IS_CLIENT_VAL="false" ;;
            *)            IS_CLIENT_VAL="true" ;;
        esac
    fi

    local ALLOWED_USER_IDS_INPUT=""
    printf "ALLOWED_USER_IDS (comma-separated, optional): "
    read ALLOWED_USER_IDS_INPUT

    local SOCKS_PROXY_ADDR_INPUT=""
    printf "SOCKS_PROXY_ADDR (optional, e.g. 10.0.0.1): "
    read SOCKS_PROXY_ADDR_INPUT

    local SOCKS_PROXY_PORT_INPUT=""
    printf "SOCKS_PROXY_PORT (optional, default: 1080): "
    read SOCKS_PROXY_PORT_INPUT

    local SOCKS_PROXY_USER_INPUT=""
    printf "SOCKS_PROXY_USER (optional): "
    read SOCKS_PROXY_USER_INPUT

    local SOCKS_PROXY_PASS_INPUT=""
    printf "SOCKS_PROXY_PASS (optional): "
    read SOCKS_PROXY_PASS_INPUT

    # Handle overwrite prompt on upgrade
    local OVERWRITE="y"
    if [ -f "$ENV_FILE" ]; then
        printf "Config exists. Overwrite? [y/N] (default: N): "
        read OVERWRITE
    fi

    case "$OVERWRITE" in
        y|Y|yes)
            cat > "$ENV_FILE" << ENV_EOF
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
ENV_EOF
            info "Env file created: $ENV_FILE"
            ;;
        *)
            info "Env file kept: $ENV_FILE"
            ;;
    esac
}

# Post-installation verification.
verify() {
    echo ""
    echo "── Verification ──────────────────────────"
    echo ""

    local FAIL=0

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
    check "Init script exists"                  "[ -f $INITD ]"
    check "Init script enabled"                 "[ -L /etc/rc.d/S99dial-up ] || [ -L /etc/rc.d/S*dial-up ]"

    if [ "$INSTALL_TYPE" != "server" ]; then
        check "sing-box config exists"            "[ -f /etc/sing-box/config.json ]"
        check "tproxy nft snippet exists"        "[ -f /etc/sing-box/tproxy.nft ]"
        check "fw4 include registered"           "uci -q get firewall.singbox_tproxy.path >/dev/null"
        check "tproxy nft chain active"          "nft list chain inet fw4 singbox_tproxy >/dev/null 2>&1"
    fi

    check "LuCI olcRTC menu exists"  "[ -f /usr/share/luci/menu.d/luci-app-olcrtc.json ]"
    check "rpcd backend exists"      "[ -x /usr/libexec/rpcd/olcrtc-bot ]"
    check "rpcd ACL exists"          "[ -f /usr/share/rpcd/acl.d/luci-app-olcrtc.json ]"
    check "LuCI bot.js exists"       "[ -f /www/luci-static/resources/view/olcrtc/bot.js ]"
    check "LuCI statusbar.js exists" "[ -f /www/luci-static/resources/view/olcrtc/statusbar.js ]"

    local OVERLAY_USAGE
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
}

# ════════════════════════════════════════════════════════════
# MAIN
# ════════════════════════════════════════════════════════════

echo ""
echo "╔══════════════════════════════════════╗"
echo "║   Install dial-up!                   ║"
echo "║   Mode:   ${INSTALL_TYPE}"
echo "╚══════════════════════════════════════╝"
echo ""

# ── Pre-flight checks ─────────────────────────────────────
command -v wget >/dev/null 2>&1 || error "wget not found"
command -v uci  >/dev/null 2>&1 || error "uci not found (not OpenWrt?)"

# ── Detect architecture ────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    aarch64) info "Architecture: $ARCH" ;;
    armv7l)  warn "Architecture: $ARCH (armv7 not tested, binary may not work)" ;;
    *)       warn "Architecture: $ARCH (not aarch64, binary may not work)" ;;
esac

# ── Detect upgrade vs fresh install ───────────────────────
IS_UPGRADE=0
if [ -f "$INITD" ]; then
    IS_UPGRADE=1
    warn "Existing installation detected, performing upgrade..."
    stop_service
    kill_olcrtc
    if [ -f "$ENV_FILE" ]; then
        cp "$ENV_FILE" "${ENV_FILE}.bak" || true
        info "Env file backed up to ${ENV_FILE}.bak"
    fi
fi

# ── Download & install binaries (atomic) ──────────────────
download_binary "$BOT_BINARY_URL" "$BOT_BIN" "bot binary"
download_binary "$OLCRTC_BINARY_URL" "$OLCRTC_BIN" "olcrtc binary"

# ── Download init script ───────────────────────────────────
log "Downloading init script..."
wget -q -O "$INITD" "$INIT_URL" || error "Failed to download init script"
chmod 755 "$INITD"
"$INITD" enable 2>/dev/null || warn "init.d enable failed"
info "Init script: $INITD (enabled)"

# ── Install packages (client only) ────────────────────────
install_packages

# ── Install sing-box config & whitelist (client only) ─────
install_singbox_config

# ── Install tproxy rules (client only) ────────────────────
install_tproxy

# ── Install LuCI app ──────────────────────────────────────
install_luci

# ── Write env file ───────────────────────────────────────
write_env

# ── Start / restart service ───────────────────────────────
if [ "$IS_UPGRADE" = "1" ]; then
    log "Restarting service..."
    "$INITD" restart 2>/dev/null || warn "Restart failed"
else
    warn "Starting service..."
    "$INITD" start 2>/dev/null || warn "Start failed (configure VK_TOKEN first)"
fi

# ── Verification ──────────────────────────────────────────
verify
