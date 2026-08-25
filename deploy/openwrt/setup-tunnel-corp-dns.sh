#!/bin/sh
# =============================================================================
# setup-tunnel-corp-dns.sh — OpenWrt: mode-following corporate DNS over a tunnel
#
# Makes sing-box resolve corporate domains through the corporate DNS server
# while keeping the query path tied to the ACTIVE selector mode:
#   * tunnel mode  (route-select -> awg):  TCP-DNS is carried over the tunnel
#     to the server side, which answers with fakeip — the client does NOT need
#     to know the corporate DNS at all
#   * proxy mode   (route-select -> proxy): TCP-DNS goes over the SOCKS tunnel
#     and is answered by the corporate DNS server directly (it must be a full
#     recursive resolver)
#
# Why `detour: route-select` instead of the tunnel outbound directly?
#   The corporate DNS is only reachable from inside the corporate network, and
#   the way to reach it differs per mode. Tying the resolver to route-select
#   makes DNS follow the mode automatically and does NOT break the proxy mode
#   of olcrtc. Also sets the selector default to the tunnel outbound so a
#   sing-box restart does not silently fall back to "direct".
#
# Safe: backs everything up, applies, verifies inside a watchdog window and
# rolls back automatically on failure. Targets the repo template
# (deploy/openwrt/sing-box-config.json, single-line yandex server).
#
# Usage:
#   CORP_DNS_SERVER=10.0.0.5 CORP_DOMAINS="corp.example intranet.example" sh setup-tunnel-corp-dns.sh
#
# Env:
#   CORP_DNS_SERVER   IP of the corporate DNS server (required)
#   CORP_DOMAINS      space-separated corporate domains (required)
#   TUNNEL_IFACE      tunnel interface                 (default: awg0)
#   TUNNEL_OUTBOUND   sing-box outbound tag for it     (default: awg)
#   SELECTOR_TAG      selector outbound tag            (default: route-select)
#   SELECTOR_DEFAULT  selector default                 (default: awg)
#   DNS_IN_PORT       sing-box dns-in listen port      (default: 5353)
#   PUBLIC_DOMAIN     public domain for watchdog       (default: ya.ru)
#   WATCHDOG_SEC      verify window before rollback    (default: 45)
#   CONFIG            sing-box config path             (default: /etc/sing-box/config.json)
#   FORCE=1           re-apply even if already applied
# =============================================================================
set -u

: "${CORP_DNS_SERVER:?CORP_DNS_SERVER is required}"
: "${CORP_DOMAINS:?CORP_DOMAINS is required}"

TUNNEL_IFACE=${TUNNEL_IFACE:-awg0}
TUNNEL_OUTBOUND=${TUNNEL_OUTBOUND:-awg}
SELECTOR_TAG=${SELECTOR_TAG:-route-select}
SELECTOR_DEFAULT=${SELECTOR_DEFAULT:-awg}
DNS_IN_PORT=${DNS_IN_PORT:-5353}
PUBLIC_DOMAIN=${PUBLIC_DOMAIN:-ya.ru}
WATCHDOG_SEC=${WATCHDOG_SEC:-45}
CONFIG=${CONFIG:-/etc/sing-box/config.json}
VERIFY_DOMAIN=${VERIFY_DOMAIN:-$(echo "$CORP_DOMAINS" | cut -d' ' -f1)}
FORCE=${FORCE:-0}

TS=$(date +%Y%m%d-%H%M%S)
LOG=/tmp/setup-corp-dns-$TS.log
CONFIG_BAK=$CONFIG.bak-$TS
DHCP_BAK=/etc/config/dhcp.bak-$TS
RESTORE=/tmp/restore-corp-dns-$TS.sh

log()  { echo "[..] $*" | tee -a "$LOG"; }
ok()   { echo "[OK] $*" | tee -a "$LOG"; }
fail() {
  echo "[ERR] $*" | tee -a "$LOG"
  if [ -f "$RESTORE" ]; then sh "$RESTORE" 2>/dev/null; fi
  echo "ROLLBACK_DONE" | tee -a "$LOG"
  exit 1
}

grep -q '"tag": "corp-dns"' "$CONFIG" 2>/dev/null && [ "$FORCE" != "1" ] && {
  echo "corp-dns already applied (FORCE=1 to re-apply)"
  exit 0
}

log "backup: $CONFIG -> $CONFIG_BAK"
cp "$CONFIG" "$CONFIG_BAK" || fail "no $CONFIG"
cp /etc/config/dhcp "$DHCP_BAK"

cat > "$RESTORE" <<EOF
#!/bin/sh
cp "$CONFIG_BAK" "$CONFIG"
cp "$DHCP_BAK" /etc/config/dhcp
/etc/init.d/sing-box restart
/etc/init.d/dnsmasq restart
echo ROLLBACK_DONE
EOF
chmod +x "$RESTORE"

# --- sing-box: corp-dns server (after the single-line yandex server) ---
awk -v srv="$CORP_DNS_SERVER" '
    /"tag": "yandex"/ && /}/ && !done {
      print;
      print "      ,";
      print "      { \"type\": \"tcp\", \"tag\": \"corp-dns\", \"server\": \"" srv "\", \"server_port\": 53, \"detour\": \"route-select\" }";
      done = 1;
      next
    }
    { print }
' "$CONFIG" > "$CONFIG.tmp" && mv "$CONFIG.tmp" "$CONFIG"
grep -q '"tag": "corp-dns"' "$CONFIG" || fail "corp-dns insert failed (yandex must be a single-line object — redeploy the template)"

# --- sing-box: corporate-domain rule (first in dns.rules) ---
JSON_DOMAINS=$(echo "$CORP_DOMAINS" | sed 's/ /", "/g')
awk -v dom="$JSON_DOMAINS" '
    /^    "rules": [$/ && !done {
      print "        { \"domain_suffix\": [" dom "], \"server\": \"corp-dns\" },";
      done = 1
    }
    { print }
' "$CONFIG" > "$CONFIG.tmp" && mv "$CONFIG.tmp" "$CONFIG"
grep -q '"server": "corp-dns"' "$CONFIG" || fail "rule insert failed"

# --- sing-box: selector default -> tunnel outbound ---
sed -i 's/"default": "direct"/"default": "'"$SELECTOR_DEFAULT"'"/' "$CONFIG"
grep -q '"default": "'"$SELECTOR_DEFAULT"'"' "$CONFIG" || fail "default change failed"

# --- dnsmasq: corporate domains -> sing-box dns-in (127.0.0.1#5353) ---
for d in $CORP_DOMAINS; do
  uci add_list dhcp.@dnsmasq[0].server="/$d/127.0.0.1#$DNS_IN_PORT"
  uci -q delete "dhcp.@dnsmasq[0].rebind_domain=$d"
  uci add_list dhcp.@dnsmasq[0].rebind_domain="$d"
done
uci commit dhcp

/etc/init.d/sing-box restart
/etc/init.d/dnsmasq restart

# --- watchdog: verify inside the window, roll back on failure ---
log "verify up to $WATCHDOG_SEC s: $VERIFY_DOMAIN via corp-dns, $PUBLIC_DOMAIN via fallback"
i=0
while [ "$i" -lt "$WATCHDOG_SEC" ]; do
  c=$(nslookup "$VERIFY_DOMAIN" 127.0.0.1 2>/dev/null | grep -c "Address:")
  p=$(nslookup "$PUBLIC_DOMAIN" 127.0.0.1 2>/dev/null | grep -c "Address:")
  if [ "$c" -ge 2 ] && [ "$p" -ge 2 ]; then
    ok "STAGE_OK (corp answers=$c, public answers=$p)"
    exit 0
  fi
  i=$((i + 1))
  sleep 1
done

echo "[ERR] VERIFY_FAIL: corp=$c public=$p — rolling back" | tee -a "$LOG"
sh "$RESTORE"
echo "ROLLBACK_DONE" | tee -a "$LOG"
exit 1
