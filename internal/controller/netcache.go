/*
[2026-07-12] :: 🚀 :: Added hasRTT/rttMs to netSnapshot; refresh() calls singbox.DelayTest() when sbStatus.Alive to cache tunnel RTT
[2026-07-10] :: 🛡️ :: Added Stop() method with dedicated stop channel — Controller.Stop() now explicitly stops the cache goroutine without relying on ctx cancellation alone; moved initial refresh into the background goroutine so Start() returns immediately; SetRoute triggers an immediate refresh so /s reflects route changes promptly
[2026-07-10] :: 🚀 :: Initial netCache: background-refreshing diagnostics cache (30s interval, atomic.Pointer for lock-free reads)
*/

package controller

import (
	"context"
	"sync/atomic"
	"time"

	"dial-up/internal/domain/logger"
	"dial-up/internal/singbox"
)

const netCacheInterval = 30 * time.Second

// netSnapshot is an immutable snapshot of network diagnostics.
type netSnapshot struct {
	pingDNS    string
	hasSingBox bool
	sbAlive    bool
	sbRoute    string
	hasRTT     bool
	rttMs      int
}

// netCache caches network diagnostics with background refresh.
type netCache struct {
	current  atomic.Pointer[netSnapshot]
	sb       singbox.Controller
	isClient bool
	l        logger.Logger
	stop     chan struct{}
}

// newNetCache creates a netCache that will refresh PingDNS and sing-box status.
func newNetCache(sb singbox.Controller, isClient bool, l logger.Logger) *netCache {
	return &netCache{
		sb:       sb,
		isClient: isClient,
		l:        l,
		stop:     make(chan struct{}),
	}
}

// Start spawns a background goroutine that refreshes diagnostics immediately and every 30s.
func (nc *netCache) Start(ctx context.Context) {
	cl := nc.l.With(logger.Function("netCache.Start"))

	go func() {
		cl.Debug("controller", "Performing initial diagnostics refresh", logger.Block("InitialRefresh"), logger.Status("ATTEMPT"), logger.Importance(4))
		nc.refresh()
		cl.Debug("controller", "Initial diagnostics refresh complete", logger.Block("InitialRefresh"), logger.Status("OK"), logger.Importance(4))

		ticker := time.NewTicker(netCacheInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				nc.refresh()
			case <-nc.stop:
				cl.Info("controller", "Diagnostics cache goroutine exiting via Stop", logger.Block("BackgroundRefresh"), logger.Status("OK"), logger.Importance(5))
				return
			case <-ctx.Done():
				cl.Info("controller", "Diagnostics cache goroutine exiting via ctx", logger.Block("BackgroundRefresh"), logger.Status("OK"), logger.Importance(5))
				return
			}
		}
	}()
}

// Stop signals the background refresh goroutine to exit.
func (nc *netCache) Stop() {
	close(nc.stop)
}

// refresh collects fresh network diagnostics and stores them atomically.
func (nc *netCache) refresh() {
	cl := nc.l.With(logger.Function("netCache.refresh"))

	snap := netSnapshot{
		pingDNS: PingDNS(),
	}

	if nc.isClient && nc.sb != nil {
		sbStatus, _ := nc.sb.Status()
		snap.hasSingBox = true
		snap.sbAlive = sbStatus.Alive
		snap.sbRoute = sbStatus.Route

		if sbStatus.Alive {
			rtt, err := nc.sb.DelayTest()
			if err == nil && rtt > 0 {
				snap.hasRTT = true
				snap.rttMs = rtt
			} else {
				cl.Debug("controller", "Delay test skipped or failed", logger.Block("Collect"), logger.Status("SKIP"), logger.Importance(3), logger.Int("rtt", rtt), logger.Error(err))
			}
		}
	}

	nc.current.Store(&snap)
	cl.Debug("controller", "Diagnostics cache refreshed", logger.Block("Collect"), logger.Status("OK"), logger.Importance(4), logger.String("ping", snap.pingDNS), logger.Bool("sbAlive", snap.sbAlive), logger.String("sbRoute", snap.sbRoute), logger.Bool("hasRTT", snap.hasRTT), logger.Int("rttMs", snap.rttMs))
}

// get returns the cached diagnostics snapshot via lock-free atomic load.
func (nc *netCache) get() netSnapshot {
	p := nc.current.Load()
	if p == nil {
		return netSnapshot{}
	}
	return *p
}
