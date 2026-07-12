/*
[2026-07-12] :: 🚀 :: Initial boot-resilience regression tests for the retryable long-poll construction in Bot.Run()
*/

package tests

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"dial-up/internal/bot"
	"dial-up/internal/config"
	"dial-up/internal/domain/logger"

	longpoll "github.com/SevereCloud/vksdk/v3/longpoll-user"
)

// Static sentinel errors so tests never create dynamic errors (err113). These simulate the
// cold-boot DNS SERVFAIL that previously killed the process from Bot.Run().
var (
	errSimulatedDNSSERVFAIL = errors.New("simulated DNS SERVFAIL: lookup api.vk.ru on [::1]:53: server misbehaving")
	errConstructLoopFail    = errors.New("keep failing to stay in construct-backoff loop")
)

// TestRunRetriesOnConstructFailure ensures construction failures are retried, not fatal.
func TestRunRetriesOnConstructFailure(t *testing.T) {
	l := logger.New(true)
	b := bot.New(nil, &fakeController{}, &config.Config{}, l)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// attempt so the subsequent interruptible backoff exits immediately via ctx.Done.
	var calls int32
	b.SetLongPollFactory(func() (*longpoll.LongPoll, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 2 {
			cancel()
		}
		return nil, errSimulatedDNSSERVFAIL
	})

	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("[LDD] Run returned non-nil error on construction failure: %v — construction MUST be retryable (pre-fix this killed the process)", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("[LDD] Run did not return within 6s — backoff may not be interruptible by ctx")
	}

	if calls < 2 {
		t.Fatalf("[LDD] expected construction to be retried (>=2 attempts), got %d — Run() must not return on a single construction failure", calls)
	}

	t.Logf("[LDD IMP:8] construction failed %d time(s) then exited via ctx; Run returned nil — self-healing confirmed (bug fix verified)", calls)
}

// TestRunExitsOnCtxCancel ensures Run() returns nil on ctx cancellation without leaking.
func TestRunExitsOnCtxCancel(t *testing.T) {
	l := logger.New(true)
	b := bot.New(nil, &fakeController{}, &config.Config{}, l)

	b.SetLongPollFactory(func() (*longpoll.LongPoll, error) {
		return nil, errConstructLoopFail
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("[LDD] Run should return nil on ctx cancel, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("[LDD] Run did not exit after ctx cancel — backoff sleep is not interruptible (goroutine leak risk)")
	}

	t.Logf("[LDD IMP:7] Run exited cleanly via ctx cancellation — no hang, no leak")
}
