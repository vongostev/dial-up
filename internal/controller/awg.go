/*
[2026-08-25] :: 🐛 :: ParseAwgHandshake matched output column 0 against the interface name, but "awg show latest-handshakes" prints the peer public key there — awg_online was always false; now parses any handshake line (the query is already per-interface)
[2026-08-24] :: 🚀 :: Initial AmneziaWG helpers: liveness (handshake), UCI enumeration, interface resolution, presence
*/

package controller

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// awgBinary is the AmneziaWG userland tool.
	awgBinary = "awg"
	// awgUciProto is the UCI network proto marking an AmneziaWG interface.
	awgUciProto = "amneziawg"
	// sysfsNetDir is where Linux exposes network devices.
	sysfsNetDir = "/sys/class/net"
	// defaultAwgHandshakeMax bounds liveness when no maxAge is supplied.
	defaultAwgHandshakeMax = 180 * time.Second
)

// AwgRunner executes an awg subcommand for an interface and returns combined output.
type AwgRunner func(ctx context.Context, iface string) (string, error)

// AwgOnline reports whether the AmneziaWG interface has a fresh handshake.
func AwgOnline(ctx context.Context, iface string, maxAge time.Duration) bool {
	return AwgOnlineWithRunner(awgShowRunner, ctx, iface, maxAge, time.Now())
}

// AwgOnlineWithRunner decides liveness from an injected awg runner.
func AwgOnlineWithRunner(run AwgRunner, ctx context.Context, iface string, maxAge time.Duration, now time.Time) bool {
	if maxAge <= 0 {
		maxAge = defaultAwgHandshakeMax
	}
	out, err := run(ctx, iface)
	if err != nil {
		return false
	}
	ts, ok := ParseAwgHandshake(out)
	if !ok || ts <= 0 {
		return false
	}
	return now.Sub(time.Unix(ts, 0)) <= maxAge
}

// awgShowRunner runs "awg show <iface> latest-handshakes".
//
//nolint:gosec // iface comes from UCI enumeration or operator config, not user input.
func awgShowRunner(ctx context.Context, iface string) (string, error) {
	out, err := exec.CommandContext(ctx, awgBinary, "show", iface, "latest-handshakes").CombinedOutput()
	return string(out), err
}

// ParseAwgHandshake extracts the latest-handshake timestamp from awg output.
// Exported for tests; used by AwgOnlineWithRunner.
func ParseAwgHandshake(out string) (int64, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ts, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return ts, true
	}
	return 0, false
}

// UciShowRunner runs "uci show network" and returns combined output.
type UciShowRunner func(ctx context.Context) (string, error)

// AwgInterfaces lists UCI-configured amneziawg interface names.
func AwgInterfaces(ctx context.Context) []string {
	return AwgInterfacesWith(func(c context.Context) (string, error) {
		out, err := exec.CommandContext(c, "uci", "show", "network").CombinedOutput()
		return string(out), err
	}, ctx)
}

// AwgInterfacesWith enumerates amneziawg interfaces from an injected uci runner.
func AwgInterfacesWith(run UciShowRunner, ctx context.Context) []string {
	out, err := run(ctx)
	if err != nil {
		return nil
	}
	return ParseAwgInterfaces(out)
}

// ParseAwgInterfaces extracts amneziawg interface names from uci show output.
// Exported for tests; used by AwgInterfacesWith.
func ParseAwgInterfaces(out string) []string {
	var res []string
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "network.")
		if !ok {
			continue
		}
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			continue
		}
		key := rest[:eq]
		val := strings.Trim(strings.TrimSpace(rest[eq+1:]), "'\"")
		if strings.HasSuffix(key, ".proto") && val == awgUciProto {
			res = append(res, strings.TrimSuffix(key, ".proto"))
		}
	}
	return res
}

// ResolveAwgIface returns the interface to use: override, the single detected one, or "".
func ResolveAwgIface(override string, detected []string) string {
	if override != "" {
		return override
	}
	if len(detected) == 1 {
		return detected[0]
	}
	return ""
}

// AwgPresent reports whether the AmneziaWG interface is configured or exists.
func AwgPresent(ctx context.Context, iface string) bool {
	if iface == "" {
		return false
	}
	return AwgPresentWith(
		sysfsIfaceExists,
		func() bool {
			out, err := exec.CommandContext(ctx, "uci", "-q", "get", "network."+iface+".proto").CombinedOutput()
			return err == nil && strings.TrimSpace(string(out)) == awgUciProto
		},
		iface,
	)
}

// AwgPresentWith decides presence from injected sysfs and uci-proto checks.
func AwgPresentWith(ifaceExists func(string) bool, protoIsAwg func() bool, iface string) bool {
	if iface == "" {
		return false
	}
	return ifaceExists(iface) || protoIsAwg()
}

// sysfsIfaceExists reports whether /sys/class/net/<iface> exists.
func sysfsIfaceExists(iface string) bool {
	_, err := os.Stat(sysfsNetDir + "/" + iface)
	return err == nil
}
