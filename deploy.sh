#!/bin/sh
# =============================================================================
# Deploy dial-up to OpenWrt router via SSH
# Usage: ./deploy.sh [client|server] [user@host]    (default: client root@192.168.1.1)
# =============================================================================

set -e

# Positional arguments: ./deploy.sh [client|server] [user@host]
TYPE="${1:-client}"
HOST="${2:-root@192.168.1.1}"
REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
SSH_OPTS="-o LogLevel=ERROR -o ConnectTimeout=5 -o StrictHostKeyChecking=no"

BINARY="$REPO_DIR/bin/dial-up-linux-arm64"
OLCRTC_BINARY="$REPO_DIR/bin/olcrtc-linux-arm64"
INIT_SCRIPT="$REPO_DIR/deploy/openwrt/init.d/dial-up"
ENV_SAMPLE="$REPO_DIR/deploy/openwrt/dial-up.env.sample"
TPROXY_SETUP="$REPO_DIR/deploy/openwrt/setup-singbox-tproxy.sh"
SINGBOX_CONFIG="$REPO_DIR/deploy/openwrt/sing-box-config.json"
WHITELIST_CONFIG="$REPO_DIR/deploy/openwrt/whitelist.json"
ROUTER_BIN="/usr/bin/dial-up"
ROUTER_OLCRTC="/etc/olcrtc-linux-arm64"
ROUTER_INIT="/etc/init.d/dial-up"
ROUTER_ENV="/etc/dial-up.env"
ROUTER_SINGBOX_CONFIG="/etc/sing-box/config.json"
ROUTER_WHITELIST_CONFIG="/etc/sing-box/whitelist.json"

case "$TYPE" in
    client) IS_CLIENT=${IS_CLIENT:-true}  ;;
    server) IS_CLIENT=${IS_CLIENT:-false} ;;
    *) printf '[ERR] Unknown type "%s" (expected: client|server)\n' "$TYPE" >&2; exit 1 ;;
esac
ALLOWED_USER_IDS=${ALLOWED_USER_IDS:-}
SOCKS_PROXY_ADDR=${SOCKS_PROXY_ADDR:-}
SOCKS_PROXY_PORT=${SOCKS_PROXY_PORT:-}
SOCKS_PROXY_USER=${SOCKS_PROXY_USER:-}
SOCKS_PROXY_PASS=${SOCKS_PROXY_PASS:-}

GREEN='\033[32m'
RED='\033[31m'
YELLOW='\033[33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { printf "${GREEN}[OK]${NC} %s\n" "$*"; }
warn()  { printf "${YELLOW}[!!]${NC} %s\n" "$*"; }
error() { printf "${RED}[ERR]${NC} %s\n" "$*"; exit 1; }
log() { printf '[..] %s\n' "$*"; }

echo ""
echo "╔══════════════════════════════════════╗"
echo "║   Deploy dial-up!                    ║"
echo "║   Type:   ${TYPE}"
echo "║   Target: ${HOST}"
echo "╚══════════════════════════════════════╝"
echo ""

# ── Pre-flight checks ─────────────────────────────────────
log "Checking SSH connectivity..."
ssh $SSH_OPTS "$HOST" "command -v uci >/dev/null && echo openwrt" 2>/dev/null | grep -q openwrt || \
    error "SSH unreachable or not OpenWRT"

[ -f "$BINARY" ] || error "Bot binary not found — run 'make build' first: $BINARY"
[ -f "$OLCRTC_BINARY" ] || error "olcrtc binary not found: $OLCRTC_BINARY"
[ -f "$INIT_SCRIPT" ] || error "Init script not found: $INIT_SCRIPT"

# ── Detect architecture ────────────────────────────────────
ARCH=$(ssh $SSH_OPTS "$HOST" "uname -m" 2>/dev/null || echo "")
case "$ARCH" in
    aarch64) info "Architecture: $ARCH (expected)" ;;
    *)       warn "Architecture: $ARCH (not aarch64, binary may not work)" ;;
esac

if [ "$IS_CLIENT" = "true" ]; then
    # ── Check & install required packages ─────────────────────
    log "Checking required packages..."
    PKGS=""
    ssh $SSH_OPTS "$HOST" "command -v sing-box >/dev/null 2>&1 || command -v sing-box-tiny >/dev/null 2>&1" || PKGS="$PKGS sing-box-tiny"
    ssh $SSH_OPTS "$HOST" "command -v setcap >/dev/null 2>&1" || PKGS="$PKGS libcap-bin"
    ssh $SSH_OPTS "$HOST" "opkg list-installed 2>/dev/null | grep -q kmod-nft-tproxy" || PKGS="$PKGS kmod-nft-tproxy"
    ssh $SSH_OPTS "$HOST" "opkg list-installed 2>/dev/null | grep -q kmod-nf-tproxy" || PKGS="$PKGS kmod-nf-tproxy"
    if [ -n "$PKGS" ]; then
        log "Installing missing packages:$PKGS"
        ssh $SSH_OPTS "$HOST" "opkg update && opkg install --force-space $PKGS" || \
            error "Failed to install required packages:$PKGS"
        info "Packages installed successfully"
    else
        info "All required packages are present"
    fi
fi

# ── Stop running service ─────────────────────────────────────
warn "Stopping service (if running)..."
ssh $SSH_OPTS "$HOST" "$ROUTER_INIT stop 2>/dev/null && while $ROUTER_INIT running >/dev/null; do sleep 1; done" || true

# ── Deploy binary ──────────────────────────────────────────
BIN_SIZE=$(stat -f%z "$BINARY" 2>/dev/null || stat -c%s "$BINARY" 2>/dev/null || echo "?")
log "Copying binary ($((BIN_SIZE / 1024)) KB)..."
cat "$BINARY" | ssh $SSH_OPTS "$HOST" "cat > $ROUTER_BIN && chmod 755 $ROUTER_BIN"
info "Binary $ROUTER_BIN ($(ssh $SSH_OPTS "$HOST" "ls -lh $ROUTER_BIN" | awk '{print $5}'))"

# ── Deploy init script ─────────────────────────────────────
log "Copying init script..."
cat "$INIT_SCRIPT" | ssh $SSH_OPTS "$HOST" "cat > $ROUTER_INIT && chmod 755 $ROUTER_INIT"
ssh $SSH_OPTS "$HOST" "$ROUTER_INIT enable 2>/dev/null" || warn "init.d enable failed"
info "Init $ROUTER_INIT (enabled)"

# ── Deploy olcrtc binary ─────────────────────────────────
warn "Killing any running olcrtc processes..."
ssh $SSH_OPTS "$HOST" "killall $ROUTER_OLCRTC 2>/dev/null; while pgrep $ROUTER_OLCRTC >/dev/null 2>&1; do sleep 1; done" || true

OLCRTC_SIZE=$(stat -f%z "$OLCRTC_BINARY" 2>/dev/null || stat -c%s "$OLCRTC_BINARY" 2>/dev/null || echo "?")
log "Copying olcrtc binary ($((OLCRTC_SIZE / 1024)) KB)..."
cat "$OLCRTC_BINARY" | ssh $SSH_OPTS "$HOST" "cat > $ROUTER_OLCRTC && chmod 755 $ROUTER_OLCRTC"
info "olcrtc $ROUTER_OLCRTC ($(ssh $SSH_OPTS "$HOST" "ls -lh $ROUTER_OLCRTC" | awk '{print $5}'))"

# ── Cleanup macOS AppleDouble files & sync UBIFS ──────────
# macOS copies may leave resource-fork artifacts (._*). Also let UBIFS
# background GC settle (sync + short sleep) so flash usage is reported
# accurately and old binary versions are reclaimed.
log "Cleaning macOS resource-fork artifacts (._*) and syncing..."
ssh $SSH_OPTS "$HOST" "find /overlay/upper -name '._*' -delete 2>/dev/null; sync; sleep 2" || true

# ── Deploy sing-box config + tproxy rules (client only) ─────
# sing-box transparent-proxy is only relevant on the client router.
# In server mode (IS_CLIENT=false) these steps are skipped entirely.
if [ "$IS_CLIENT" = "true" ]; then
    # ── Check & install required packages ─────────────────────
    log "Checking required packages..."
    PKGS=""
    ssh $SSH_OPTS "$HOST" "command -v sing-box >/dev/null 2>&1 || command -v sing-box-tiny >/dev/null 2>&1" || PKGS="$PKGS sing-box-tiny"
    ssh $SSH_OPTS "$HOST" "command -v setcap >/dev/null 2>&1" || PKGS="$PKGS libcap-bin"
    ssh $SSH_OPTS "$HOST" "opkg list-installed 2>/dev/null | grep -q kmod-nft-tproxy" || PKGS="$PKGS kmod-nft-tproxy"
    ssh $SSH_OPTS "$HOST" "opkg list-installed 2>/dev/null | grep -q kmod-nf-tproxy" || PKGS="$PKGS kmod-nf-tproxy"
    if [ -n "$PKGS" ]; then
        log "Installing missing packages:$PKGS"
        ssh $SSH_OPTS "$HOST" "opkg update && opkg install --force-space $PKGS" || \
            error "Failed to install required packages:$PKGS"
        info "Packages installed successfully"
    else
        info "All required packages are present"
    fi

    if [ -f "$SINGBOX_CONFIG" ]; then
        log "Deploying sing-box config..."
        ssh $SSH_OPTS "$HOST" "mkdir -p /etc/sing-box"
        cat "$SINGBOX_CONFIG" | ssh $SSH_OPTS "$HOST" "cat > $ROUTER_SINGBOX_CONFIG"
        info "sing-box config: $ROUTER_SINGBOX_CONFIG"
        cat "$WHITELIST_CONFIG" | ssh $SSH_OPTS "$HOST" "cat > $ROUTER_WHITELIST_CONFIG"
        info "sing-box whitelist: $ROUTER_WHITELIST_CONFIG"
    else
        warn "sing-box-config.json not found, skipping"
    fi

    if [ -f "$TPROXY_SETUP" ]; then
        warn "Running tproxy setup..."
        cat "$TPROXY_SETUP" | ssh $SSH_OPTS "$HOST" "sh -s"
    else
        warn "setup-singbox-tproxy.sh not found, skipping"
    fi
else
    info "Server mode (IS_CLIENT=false): skipping sing-box config and tproxy rules"
fi

# ── Deploy LuCI app ────────────────────────────────────────
log "Deploying LuCI olcRTC app..."
# Ensure target directories exist
ssh $SSH_OPTS "$HOST" "mkdir -p /usr/libexec/rpcd /usr/share/rpcd/acl.d /usr/share/luci/menu.d /www/luci-static/resources/view/olcrtc"
# rpcd backend
cat "$REPO_DIR/luci-app-olcrtc/root/usr/libexec/rpcd/olcrtc-bot" | \
    ssh $SSH_OPTS "$HOST" "cat > /usr/libexec/rpcd/olcrtc-bot && chmod 755 /usr/libexec/rpcd/olcrtc-bot"
# ACL
cat "$REPO_DIR/luci-app-olcrtc/root/usr/share/rpcd/acl.d/luci-app-olcrtc.json" | \
    ssh $SSH_OPTS "$HOST" "cat > /usr/share/rpcd/acl.d/luci-app-olcrtc.json"
# Menu
cat "$REPO_DIR/luci-app-olcrtc/root/usr/share/luci/menu.d/luci-app-olcrtc.json" | \
    ssh $SSH_OPTS "$HOST" "cat > /usr/share/luci/menu.d/luci-app-olcrtc.json"
# JS views (4 planes + shared statusbar module + network subviews)
for f in statusbar control data network network_client network_server logs; do
    cat "$REPO_DIR/luci-app-olcrtc/htdocs/luci-static/resources/view/olcrtc/$f.js" | \
        ssh $SSH_OPTS "$HOST" "cat > /www/luci-static/resources/view/olcrtc/$f.js"
done
# Restart rpcd to pick up new backend
ssh $SSH_OPTS "$HOST" "/etc/init.d/rpcd restart" || warn "rpcd restart failed"
info "LuCI olcRTC app deployed"

# ── Deploy env config ─────────────────────────────────────
ENV_EXISTS=$(ssh $SSH_OPTS "$HOST" "[ -f $ROUTER_ENV ] && echo yes || echo no")
if [ "$ENV_EXISTS" = "no" ]; then
    if [ -n "$VK_TOKEN" ] && [ -n "$OLCRTC_KEY" ]; then
        ssh $SSH_OPTS "$HOST" "cat > $ROUTER_ENV" <<EOF
VK_TOKEN=$VK_TOKEN
OLCRTC_KEY=$OLCRTC_KEY
IS_CLIENT=$IS_CLIENT
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
EOF
        info "Env $ROUTER_ENV (created with VK_TOKEN and OLCRTC_KEY)"
    elif [ -n "$VK_TOKEN" ] || [ -n "$OLCRTC_KEY" ]; then
        warn "VK_TOKEN or OLCRTC_KEY set but not both — falling back to sample"
        cat "$ENV_SAMPLE" | ssh $SSH_OPTS "$HOST" "cat > $ROUTER_ENV"
        warn "Edit $ROUTER_ENV with your VK_TOKEN and OLCRTC_KEY"
    else
        cat "$ENV_SAMPLE" | ssh $SSH_OPTS "$HOST" "cat > $ROUTER_ENV"
        info "Env $ROUTER_ENV (created from sample)"
        warn "Edit $ROUTER_ENV with your VK_TOKEN and OLCRTC_KEY"
    fi
else
    warn "Env $ROUTER_ENV (already exists, skipped)"
fi

# ── Restart service ────────────────────────────────────────
log "Restarting service..."
ssh $SSH_OPTS "$HOST" "$ROUTER_INIT restart 2>/dev/null" || warn "Restart failed"

# Clean macos artifacts
ssh $SSH_OPTS "$HOST" "rm -f /overlay/upper/._* /usr/bin/._* /etc/._*" 2>/dev/null || true

# ══════════════════════════════════════════════════════════
# VERIFICATION
# ══════════════════════════════════════════════════════════
echo ""
echo "── Verification ──────────────────────────"
echo ""

FAIL=0

check() {
    local desc="$1" cmd="$2"
    if ssh $SSH_OPTS "$HOST" "$cmd" >/dev/null 2>&1; then
        info "$desc"
    else
        printf "${RED}[ERR]${NC} %s\n" "$desc"
        FAIL=1
    fi
}

check "Bot binary exists and executable"  "[ -x $ROUTER_BIN ]"
check "olcrtc binary exists and executable" "[ -x $ROUTER_OLCRTC ]"
check "Init script exists"                "[ -f $ROUTER_INIT ]"
check "Init script enabled"           "[ -L /etc/rc.d/S99dial-up ] || [ -L /etc/rc.d/S*dial-up ]"
if [ "$IS_CLIENT" = "true" ]; then
    check "sing-box config exists"        "[ -f $ROUTER_SINGBOX_CONFIG ]"
    check "tproxy nft snippet exists"     "[ -f /etc/sing-box/tproxy.nft ]"
    check "fw4 include registered"        "uci -q get firewall.singbox_tproxy.path >/dev/null"
    check "tproxy nft chain active"       "nft list chain inet fw4 singbox_tproxy >/dev/null 2>&1"
fi
check "LuCI olcRTC menu exists"       "[ -f /usr/share/luci/menu.d/luci-app-olcrtc.json ]"
check "rpcd backend exists"           "[ -x /usr/libexec/rpcd/olcrtc-bot ]"
check "rpcd ACL exists"               "[ -f /usr/share/rpcd/acl.d/luci-app-olcrtc.json ]"
check "LuCI control.js exists"        "[ -f /www/luci-static/resources/view/olcrtc/control.js ]"
check "LuCI statusbar.js exists"      "[ -f /www/luci-static/resources/view/olcrtc/statusbar.js ]"

# ── Flash usage (physical, post-GC) ─────────────────────────
# df reports the actual compressed footprint on UBIFS, unlike logical sizes.
OVERLAY_USAGE=$(ssh $SSH_OPTS "$HOST" "df -h /overlay | awk 'NR==2{print \$3\" / \"\$2\" (\"\$5\" used)\"}'")
info "Overlay flash: $OVERLAY_USAGE"

echo ""
if [ "$FAIL" = "0" ]; then
    printf "${GREEN}╔══════════════════════════════════════╗${NC}\n"
    printf "${GREEN}║  Deploy successful!                  ║${NC}\n"
    printf "${GREEN}╚══════════════════════════════════════╝${NC}\n"
else
    printf "${RED}Some checks failed!${NC}\n"
    exit 1
fi
echo ""