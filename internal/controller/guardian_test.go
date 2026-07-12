/*
[2026-07-10] :: 🚀 :: Initial white-box guardian test suite: 6 scenarios covering P1B flush/restore + P3 demote
*/

package controller

import (
	"context"
	"net"
	"testing"
	"time"

	"dial-up/internal/domain/logger"
	"dial-up/internal/singbox"
)

// fakeFW is a recording firewall.Manager fake for guardian tests.
type fakeFW struct {
	flushed   bool
	flushCnt  int
	reloadCnt int
	present   bool
	listErr   error
}

func (f *fakeFW) FlushTproxy(_ context.Context) error {
	f.flushed = true
	f.flushCnt++
	return nil
}

func (f *fakeFW) ReloadFw4(_ context.Context) error {
	f.flushed = false
	f.reloadCnt++
	return nil
}

func (f *fakeFW) TproxyRulesPresent(_ context.Context) (bool, error) {
	return f.present, f.listErr
}

// fakeSB is a recording singbox.Controller fake for guardian tests.
type fakeSB struct {
	alive    bool
	route    string
	setRoute string
	setCnt   int
}

func (f *fakeSB) Status() (singbox.Status, error) {
	return singbox.Status{Alive: f.alive, Route: f.route}, nil
}

func (f *fakeSB) SetRoute(mode string) error {
	f.setRoute = mode
	f.setCnt++
	return nil
}

func (f *fakeSB) DelayTest() (int, error) { return 0, nil }

func TestGuardianAliveDirectNoOp(t *testing.T) {
	l := logger.New(true)
	sb := &fakeSB{alive: true, route: singbox.ModeDirect}
	fw := &fakeFW{}
	g := newTproxyGuardian(sb, fw, l, nil)

	g.check(context.Background())

	if fw.flushCnt != 0 {
		t.Errorf("expected 0 flushes, got %d", fw.flushCnt)
	}
	if fw.reloadCnt != 0 {
		t.Errorf("expected 0 reloads, got %d", fw.reloadCnt)
	}
	if g.flushed {
		t.Error("expected flushed=false")
	}
	if sb.setCnt != 0 {
		t.Errorf("expected 0 SetRoute calls in direct mode, got %d", sb.setCnt)
	}
}

func TestGuardianDeadFirstTickSetsDownSince(t *testing.T) {
	l := logger.New(true)
	sb := &fakeSB{alive: false}
	fw := &fakeFW{}
	g := newTproxyGuardian(sb, fw, l, nil)

	g.check(context.Background())

	if g.downSince.IsZero() {
		t.Error("expected downSince to be set on first dead tick")
	}
	if g.flushed {
		t.Error("expected flushed=false on first dead tick (threshold not elapsed)")
	}
	if fw.flushCnt != 0 {
		t.Errorf("expected 0 flushes on first dead tick, got %d", fw.flushCnt)
	}
}

func TestGuardianDeadPastThresholdFlushes(t *testing.T) {
	l := logger.New(true)
	sb := &fakeSB{alive: false}
	fw := &fakeFW{}
	g := newTproxyGuardian(sb, fw, l, nil)
	// Simulate that death was detected 15s ago — past the 10s threshold.
	g.downSince = time.Now().Add(-15 * time.Second)

	g.check(context.Background())

	if fw.flushCnt != 1 {
		t.Errorf("expected 1 flush after threshold, got %d", fw.flushCnt)
	}
	if !g.flushed {
		t.Error("expected flushed=true after flush")
	}
}

func TestGuardianRecoveryReloads(t *testing.T) {
	l := logger.New(true)
	sb := &fakeSB{alive: true, route: singbox.ModeDirect}
	fw := &fakeFW{}
	g := newTproxyGuardian(sb, fw, l, nil)
	// Pretend we previously flushed the chain.
	g.flushed = true
	g.downSince = time.Now().Add(-30 * time.Second)

	g.check(context.Background())

	if fw.reloadCnt != 1 {
		t.Errorf("expected 1 reload on recovery, got %d", fw.reloadCnt)
	}
	if g.flushed {
		t.Error("expected flushed=false after successful reload")
	}
	if !g.downSince.IsZero() {
		t.Error("expected downSince reset to zero on recovery")
	}
}

func TestGuardianProxyPortOpenNoDemote(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	l := logger.New(true)
	sb := &fakeSB{alive: true, route: singbox.ModeProxy}
	fw := &fakeFW{}
	g := newTproxyGuardian(sb, fw, l, nil)
	g.socksAddr = ln.Addr().String()

	g.check(context.Background())

	if sb.setCnt != 0 {
		t.Errorf("expected 0 SetRoute calls when SOCKS port open, got %d", sb.setCnt)
	}
}

func TestGuardianProxyPortClosedDemotes(t *testing.T) {
	// Bind to an ephemeral port then close it so DialTimeout fails fast with "connection refused".
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	closedAddr := ln.Addr().String()
	_ = ln.Close()

	l := logger.New(true)
	sb := &fakeSB{alive: true, route: singbox.ModeProxy}
	fw := &fakeFW{}
	demoted := false
	g := newTproxyGuardian(sb, fw, l, func() { demoted = true })
	g.socksAddr = closedAddr

	g.check(context.Background())

	if sb.setCnt != 1 {
		t.Errorf("expected 1 SetRoute call when SOCKS port closed, got %d", sb.setCnt)
	}
	if sb.setRoute != singbox.ModeDirect {
		t.Errorf("expected SetRoute(\"direct\"), got %q", sb.setRoute)
	}
	if !demoted {
		t.Error("expected onDemote callback to fire after demotion")
	}
}
