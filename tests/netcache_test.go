/*
[2026-07-12] :: 🚀 :: Added TestNetCacheRTTPropagation and TestNetCacheRTTDeadSb — verify DelayTest RTT propagates into Status.TunnelRTTMs and dead sb skips the call
[2026-07-10] :: 🛡️ :: Updated tests for async boot, added TestNetCacheStopTerminatesGoroutine and TestSetRouteRefreshesCache
[2026-07-10] :: 🚀 :: Initial netCache test suite
*/

package tests

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"dial-up/internal/config"
	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
	"dial-up/internal/singbox"
)

// slowFakeSingBox implements singbox.Controller with a configurable delay on Status().
type slowFakeSingBox struct {
	delay    time.Duration
	status   singbox.Status
	calls    atomic.Int64
	rtt      int
	rttErr   error
	rttCalls atomic.Int64
}

func (s *slowFakeSingBox) SetRoute(_ string) error { return nil }

func (s *slowFakeSingBox) Status() (singbox.Status, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.status, nil
}

func (s *slowFakeSingBox) DelayTest() (int, error) {
	s.rttCalls.Add(1)
	return s.rtt, s.rttErr
}

// pollStatus waits for a condition on controller.Status to become true, with a timeout.
func pollStatus(t *testing.T, ctrl *controller.Controller, timeout time.Duration, cond func(controller.Status) bool) controller.Status {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s := ctrl.Status()
		if cond(s) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pollStatus: condition not met within timeout")
	return controller.Status{}
}

func TestNetCacheRefresh(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)

	// Initial refresh is now async — poll until the cache is populated.
	s := pollStatus(t, ctrl, 5*time.Second, func(s controller.Status) bool {
		return s.PingDNS != "" && s.SingBoxAlive != nil
	})

	t.Logf("PingDNS=%q, SingBoxAlive=%v, SingBoxRoute=%q", s.PingDNS, s.SingBoxAlive, s.SingBoxRoute)

	if !*s.SingBoxAlive {
		t.Error("expected SingBoxAlive=true")
	}
	if s.SingBoxRoute != singbox.ModeProxy {
		t.Errorf("SingBoxRoute=%q, want proxy", s.SingBoxRoute)
	}
	if sb.calls.Load() < 1 {
		t.Error("expected at least 1 sing-box Status() call")
	}
	t.Logf("sing-box Status() calls: %d", sb.calls.Load())
}

func TestNetCacheServerModeSkipsSingBox(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
	}

	cfg := &config.Config{IsClient: false, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)

	// Wait for the async PingDNS refresh to complete (server mode skips sing-box).
	s := pollStatus(t, ctrl, 5*time.Second, func(s controller.Status) bool {
		return s.PingDNS != ""
	})

	if s.SingBoxAlive != nil {
		t.Errorf("expected SingBoxAlive nil in server mode, got %v", s.SingBoxAlive)
	}
	if sb.calls.Load() != 0 {
		t.Errorf("expected 0 sing-box calls in server mode, got %d", sb.calls.Load())
	}
}

func TestSnapshotFastWithSlowSingBox(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		delay:  2 * time.Second,
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start is now async — it should return immediately even with a 2s slow sing-box.
	startTime := time.Now()
	ctrl.Start(ctx)
	bootDuration := time.Since(startTime)
	t.Logf("Boot (async, no blocking) took: %v", bootDuration)

	if bootDuration > 50*time.Millisecond {
		t.Errorf("Start() took %v, expected <50ms (async boot should not block on I/O)", bootDuration)
	}

	// Wait for the async initial refresh to complete (takes ~2s due to slow sing-box).
	s := pollStatus(t, ctrl, 5*time.Second, func(s controller.Status) bool {
		return s.SingBoxAlive != nil
	})
	t.Logf("Cache populated: PingDNS=%q, SBAlive=%v, SBRoute=%q", s.PingDNS, s.SingBoxAlive, s.SingBoxRoute)

	// Now the cache is populated. Subsequent Status() calls should be near-instant.
	for i := range 5 {
		reqStart := time.Now()
		s := ctrl.Status()
		reqDuration := time.Since(reqStart)

		if reqDuration > 50*time.Millisecond {
			t.Errorf("Status() call %d took %v, expected <50ms (cache should serve without I/O)", i, reqDuration)
		}

		if s.SingBoxAlive == nil || !*s.SingBoxAlive {
			t.Errorf("call %d: expected SingBoxAlive=true from cache", i)
		}
		if s.SingBoxRoute != singbox.ModeProxy {
			t.Errorf("call %d: SingBoxRoute=%q, want proxy", i, s.SingBoxRoute)
		}

		t.Logf("Status() call %d: %v", i, reqDuration)
	}
}

func TestNetCacheStopTerminatesGoroutine(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	// Use a context that is NOT cancelled — we rely on Stop() alone to stop the goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl.Start(ctx)

	// Wait for the initial async refresh.
	pollStatus(t, ctrl, 5*time.Second, func(s controller.Status) bool {
		return s.PingDNS != ""
	})

	callsBeforeStop := sb.calls.Load()
	t.Logf("sing-box calls before Stop: %d", callsBeforeStop)

	// Stop the controller — this should stop the netCache goroutine.
	ctrl.Stop()

	// Wait long enough to confirm no further refresh happens.
	time.Sleep(200 * time.Millisecond)

	callsAfterStop := sb.calls.Load()
	t.Logf("sing-box calls after Stop+200ms: %d", callsAfterStop)

	if callsAfterStop > callsBeforeStop {
		t.Errorf("expected no further sing-box calls after Stop, got %d→%d", callsBeforeStop, callsAfterStop)
	}
}

func TestNetCacheStartExitsOnCancel(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	ctx, cancel := context.WithCancel(context.Background())
	ctrl.Start(ctx)

	// Wait for the async initial refresh to populate the cache.
	pollStatus(t, ctrl, 5*time.Second, func(s controller.Status) bool {
		return s.PingDNS != ""
	})

	cancel()

	time.Sleep(100 * time.Millisecond)

	s := ctrl.Status()
	if s.PingDNS == "" {
		t.Error("expected cached PingDNS to survive ctx cancel")
	}
}

func TestNetCacheGetBeforeRefresh(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	// Do NOT call Start — no refresh has run.
	s := ctrl.Status()

	if s.PingDNS != "" {
		t.Errorf("expected empty PingDNS before refresh, got %q", s.PingDNS)
	}
	if s.SingBoxAlive != nil {
		t.Errorf("expected nil SingBoxAlive before refresh, got %v", s.SingBoxAlive)
	}
}

func TestSetRouteRefreshesCache(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)

	// Wait for the initial async refresh to complete (at least 1 sing-box call).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && sb.calls.Load() < 1 {
		time.Sleep(20 * time.Millisecond)
	}
	if sb.calls.Load() < 1 {
		t.Fatal("expected initial refresh to produce at least 1 sing-box call")
	}
	t.Logf("Initial cache populated, sb.calls=%d", sb.calls.Load())

	// SetRoute should trigger an async cache refresh.
	if err := ctrl.SetRoute(singbox.ModeDirect); err != nil {
		t.Fatalf("SetRoute failed: %v", err)
	}

	// Poll for the SetRoute-triggered refresh to produce a 2nd sing-box call.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && sb.calls.Load() < 2 {
		time.Sleep(20 * time.Millisecond)
	}

	if sb.calls.Load() < 2 {
		t.Errorf("expected ≥2 sing-box Status() calls (initial + SetRoute refresh), got %d", sb.calls.Load())
	} else {
		t.Logf("SetRoute triggered refresh confirmed, sb.calls=%d", sb.calls.Load())
	}
}

func TestNetCacheRTTPropagation(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: true, Route: singbox.ModeProxy},
		rtt:    42,
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)

	s := pollStatus(t, ctrl, 5*time.Second, func(s controller.Status) bool {
		return s.TunnelRTTMs != nil
	})

	if *s.TunnelRTTMs != 42 {
		t.Errorf("TunnelRTTMs = %d, want 42", *s.TunnelRTTMs)
	}
	if sb.rttCalls.Load() < 1 {
		t.Errorf("expected at least 1 DelayTest call, got %d", sb.rttCalls.Load())
	}
	t.Logf("TunnelRTTMs=%d, rttCalls=%d", *s.TunnelRTTMs, sb.rttCalls.Load())
}

func TestNetCacheRTTDeadSb(t *testing.T) {
	l := logger.New(true)
	sb := &slowFakeSingBox{
		status: singbox.Status{Alive: false},
	}

	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, sb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)

	s := pollStatus(t, ctrl, 5*time.Second, func(s controller.Status) bool {
		return s.PingDNS != ""
	})

	if s.TunnelRTTMs != nil {
		t.Errorf("expected nil TunnelRTTMs when sing-box is dead, got %d", *s.TunnelRTTMs)
	}
	if sb.rttCalls.Load() != 0 {
		t.Errorf("expected 0 DelayTest calls when sb is dead, got %d", sb.rttCalls.Load())
	}
	t.Logf("TunnelRTTMs=%v, rttCalls=%d", s.TunnelRTTMs, sb.rttCalls.Load())
}
