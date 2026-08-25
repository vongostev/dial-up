/*
[2026-08-25] :: 🐛 :: Tests now use the real awg output format ("<peer_pubkey>\t<ts>"); previously asserted "<iface>\t<ts>" which never occurs
[2026-08-24] :: 🚀 :: Initial AWG helper tests
*/

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"dial-up/internal/controller"
)

func TestParseAwgHandshake(t *testing.T) {
	// Real "awg show <iface> latest-handshakes" output: one line per peer,
	// "<peer_pubkey>\t<unix_ts>" — column 0 is the peer key, never the iface.
	out := "ZILhx7CNrxaQ2KZS0bxEJNIb6syg3HdVbaUY7XXZIwg=\t1234567\n"

	ts, ok := controller.ParseAwgHandshake(out)
	if !ok || ts != 1234567 {
		t.Errorf("handshake = (%d, %v), want (1234567, true)", ts, ok)
	}
	// ts=0 means "never handshaken": parsed ok, caller treats ts<=0 as offline.
	ts, ok = controller.ParseAwgHandshake("ZILhx7CNrxaQ2KZS0bxEJNIb6syg3HdVbaUY7XXZIwg=\t0\n")
	if !ok || ts != 0 {
		t.Errorf("never handshaken = (%d, %v), want (0, true)", ts, ok)
	}
	if _, ok := controller.ParseAwgHandshake("ZILhx7CNrxaQ2KZS0bxEJNIb6syg3HdVbaUY7XXZIwg=\tnotanumber\n"); ok {
		t.Error("expected ok=false for malformed timestamp")
	}
	if _, ok := controller.ParseAwgHandshake(""); ok {
		t.Error("expected ok=false for empty output")
	}
}

func TestAwgOnlineWithRunner(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cases := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		{name: "fresh", out: "ZILhx7CNrxaQ2KZS0bxEJNIb6syg3HdVbaUY7XXZIwg=\t1699999900\n", want: true},
		{name: "stale", out: "ZILhx7CNrxaQ2KZS0bxEJNIb6syg3HdVbaUY7XXZIwg=\t1699999000\n", want: false},
		{name: "never", out: "ZILhx7CNrxaQ2KZS0bxEJNIb6syg3HdVbaUY7XXZIwg=\t0\n", want: false},
		{name: "runner error", err: errors.New("boom"), want: false},
		{name: "empty", out: "", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := func(_ context.Context, _ string) (string, error) { return c.out, c.err }
			if got := controller.AwgOnlineWithRunner(run, context.Background(), "awg0", 180*time.Second, now); got != c.want {
				t.Errorf("AwgOnlineWithRunner = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseAwgInterfaces(t *testing.T) {
	out := "network.loopback=interface\nnetwork.lan=interface\nnetwork.awg0=interface\nnetwork.awg0.proto='amneziawg'\nnetwork.lan.proto='static'\nnetwork.awg1=interface\nnetwork.awg1.proto=\"amneziawg\"\n"
	got := controller.ParseAwgInterfaces(out)
	if len(got) != 2 || got[0] != "awg0" || got[1] != "awg1" {
		t.Errorf("ParseAwgInterfaces = %v, want [awg0 awg1]", got)
	}
	if got := controller.ParseAwgInterfaces(""); len(got) != 0 {
		t.Errorf("ParseAwgInterfaces(empty) = %v, want []", got)
	}
	if got := controller.ParseAwgInterfaces("network.x.proto='static'\n"); len(got) != 0 {
		t.Errorf("ParseAwgInterfaces(static) = %v, want []", got)
	}
}

func TestResolveAwgIface(t *testing.T) {
	if got := controller.ResolveAwgIface("awg1", []string{"awg0", "awg1"}); got != "awg1" {
		t.Errorf("override = %q, want awg1", got)
	}
	if got := controller.ResolveAwgIface("", []string{"awg0"}); got != "awg0" {
		t.Errorf("single = %q, want awg0", got)
	}
	if got := controller.ResolveAwgIface("", nil); got != "" {
		t.Errorf("none = %q, want empty", got)
	}
	if got := controller.ResolveAwgIface("", []string{"awg0", "awg1"}); got != "" {
		t.Errorf("several = %q, want empty (manual selection required)", got)
	}
}

func TestAwgPresentWith(t *testing.T) {
	if !controller.AwgPresentWith(func(string) bool { return true }, func() bool { return false }, "awg0") {
		t.Error("expected true when sysfs exists")
	}
	if !controller.AwgPresentWith(func(string) bool { return false }, func() bool { return true }, "awg0") {
		t.Error("expected true when uci proto is amneziawg")
	}
	if controller.AwgPresentWith(func(string) bool { return false }, func() bool { return false }, "awg0") {
		t.Error("expected false when neither check passes")
	}
	if controller.AwgPresentWith(func(string) bool { return true }, func() bool { return true }, "") {
		t.Error("expected false for empty interface name")
	}
}
