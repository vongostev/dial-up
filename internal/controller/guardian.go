/*
[2026-07-10] :: 🚀 :: Initial tproxyGuardian: 10s health watchdog coupling nft tproxy chain to sing-box liveness (flush/reload) + proxy-mode demote on dead :1080
*/

package controller

import (
	"context"
	"time"

	"dial-up/internal/domain/logger"
	"dial-up/internal/firewall"
	"dial-up/internal/singbox"
)

const (
	// guardianInterval is the watchdog tick period.
	guardianInterval = 10 * time.Second
	// singboxDownThreshold is how long sing-box must be dead before the tproxy chain is flushed.
	singboxDownThreshold = 10 * time.Second
	// socksCheckAddr is the olcrtc SOCKS5 listen address checked during the proxy-mode race window.
	socksCheckAddr = "127.0.0.1:1080"
	// socksCheckTimeout is the TCP connect timeout for the olcrtc SOCKS liveness check.
	socksCheckTimeout = 1 * time.Second
)

// tproxyGuardian couples the nft tproxy chain to sing-box health on a 10s ticker.
type tproxyGuardian struct {
	sb        singbox.Controller
	fw        firewall.Manager
	l         logger.Logger
	downSince time.Time
	flushed   bool
	onDemote  func()
	socksAddr string
	stop      chan struct{}
}

// newTproxyGuardian creates a tproxyGuardian with the default interval, threshold, and SOCKS address.
func newTproxyGuardian(sb singbox.Controller, fw firewall.Manager, l logger.Logger, onDemote func()) *tproxyGuardian {
	return &tproxyGuardian{
		sb:        sb,
		fw:        fw,
		l:         l,
		onDemote:  onDemote,
		socksAddr: socksCheckAddr,
		stop:      make(chan struct{}),
	}
}

// Start spawns the background watchdog goroutine.
func (g *tproxyGuardian) Start(ctx context.Context) {
	cl := g.l.With(logger.Function("tproxyGuardian.Start"))

	go func() {
		// Handles the bot-restart-after-flush edge case: if the chain was already flushed
		// before the bot (re)started, the guardian learns that so it can reload on recovery.
		present, err := g.fw.TproxyRulesPresent(ctx)
		if err != nil {
			cl.Warn("controller", "Failed to sync tproxy chain state on startup, assuming not flushed", logger.Block("SyncFlushed"), logger.Status("FAIL"), logger.Importance(5), logger.Error(err))
			g.flushed = false
		} else {
			g.flushed = !present
			cl.Debug("controller", "Synced flushed flag from chain state", logger.Block("SyncFlushed"), logger.Status("OK"), logger.Importance(4), logger.Bool("present", present), logger.Bool("flushed", g.flushed))
		}

		g.check(ctx)

		ticker := time.NewTicker(guardianInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				g.check(ctx)
			case <-g.stop:
				cl.Info("controller", "Guardian goroutine exiting via Stop", logger.Block("Watchdog"), logger.Status("OK"), logger.Importance(5))
				return
			case <-ctx.Done():
				cl.Info("controller", "Guardian goroutine exiting via ctx", logger.Block("Watchdog"), logger.Status("OK"), logger.Importance(5))
				return
			}
		}
	}()
}

// Stop signals the background watchdog goroutine to exit.
func (g *tproxyGuardian) Stop() {
	close(g.stop)
}

// check performs one health-coupling tick: flush/restore the tproxy chain and demote on dead SOCKS.
func (g *tproxyGuardian) check(ctx context.Context) {
	cl := g.l.With(logger.Function("tproxyGuardian.check"))

	sbStatus, _ := g.sb.Status()
	cl.Debug("controller", "Sing-box status snapshot", logger.Block("Status"), logger.Status("ATTEMPT"), logger.Importance(4), logger.Bool("alive", sbStatus.Alive), logger.String("route", sbStatus.Route))

	// (and not already flushed) empty the chain so LAN bypasses the dead sing-box.
	if !sbStatus.Alive {
		if g.downSince.IsZero() {
			g.downSince = time.Now()
			cl.Warn("controller", "Sing-box down detected, starting death timer", logger.Block("FlushDecision"), logger.Status("ATTEMPT"), logger.Importance(6))
		} else if time.Since(g.downSince) >= singboxDownThreshold && !g.flushed {
			cl.Warn("controller", "Sing-box down past threshold, flushing tproxy chain", logger.Block("FlushDecision"), logger.Status("ATTEMPT"), logger.Importance(8), logger.Any("downFor", time.Since(g.downSince).String()))
			if err := g.fw.FlushTproxy(ctx); err != nil {
				cl.Error("controller", "Failed to flush tproxy chain, will retry next tick", logger.Block("FlushDecision"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err))
			} else {
				g.flushed = true
				cl.Warn("controller", "Tproxy chain flushed — LAN traffic now bypasses dead sing-box", logger.Block("FlushDecision"), logger.Status("OK"), logger.Importance(8))
			}
		}
	} else {
		// repopulate the tproxy chain, then reset the death tracker.
		if g.flushed {
			cl.Info("controller", "Sing-box recovered, reloading fw4 to restore tproxy chain", logger.Block("RestoreDecision"), logger.Status("ATTEMPT"), logger.Importance(8))
			if err := g.fw.ReloadFw4(ctx); err != nil {
				cl.Error("controller", "Failed to reload fw4, will retry next tick", logger.Block("RestoreDecision"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err))
			} else {
				g.flushed = false
				cl.Info("controller", "fw4 reloaded — tproxy chain restored", logger.Block("RestoreDecision"), logger.Status("OK"), logger.Importance(7))
			}
		}
		g.downSince = time.Time{}
	}

	// If olcrtc's SOCKS listener is unreachable while sing-box still proxies to it, demote to
	// "direct" so LAN keeps working, and refresh the netCache so /s reflects the demotion.
	if sbStatus.Alive && sbStatus.Route == singbox.ModeProxy {
		if !PortOpen(g.socksAddr, socksCheckTimeout) {
			cl.Warn("controller", "olcrtc SOCKS port dead while selector=proxy, demoting to direct", logger.Block("PortCheckDemote"), logger.Status("ATTEMPT"), logger.Importance(8), logger.String("addr", g.socksAddr))
			if err := g.sb.SetRoute(singbox.ModeDirect); err != nil {
				cl.Error("controller", "Failed to demote selector to direct", logger.Block("PortCheckDemote"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err))
			} else {
				cl.Info("controller", "Selector demoted to direct", logger.Block("PortCheckDemote"), logger.Status("OK"), logger.Importance(7))
			}
			if g.onDemote != nil {
				g.onDemote()
			}
		} else {
			cl.Debug("controller", "olcrtc SOCKS port alive, no demotion needed", logger.Block("PortCheckDemote"), logger.Status("OK"), logger.Importance(4))
		}
	}
}
