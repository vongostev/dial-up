/*
[2026-07-10] :: 🚀 :: Added tproxyGuardian wiring: New() creates it (client-mode only) with firewall.New + netCache.refresh callback; Start() starts it after netCache; Stop() stops it before process kill — couples nft tproxy chain to sing-box liveness (flush/reload) and demotes selector on dead olcrtc SOCKS :1080
[2026-07-10] :: 🚀 :: loop() now calls RenderConfig(RenderParams{...}) passing SOCKS_PROXY_* fields for server-mode egress
[2026-07-10] :: 🚀 :: Replaced synchronous PingDNS + singbox.Status calls in snapshot() with a background netCache (30s refresh, atomic.Pointer reads) — /s and /status now respond in microseconds instead of 2-4s
[2026-07-08] :: 🚀 :: Added PingDNS, SingBoxAlive, SingBoxRoute fields to Status struct; snapshot() gathers network diagnostics outside the mutex
[2026-07-07] :: 🐛 :: Fixed latent data race in Backoff log: snapshot c.failures under the lock (was read after mu.Unlock, racing with waitCmd/SetProvider/ClearProvider)
[2026-07-07] :: 🐛 :: Reset crashFailures in SetProvider (new provider) and the stable>30s recovery branch — otherwise the maxFailures safety net could prematurely remove a recoverable/flapping provider or a brand-new provider inheriting a prior provider's crash count
[2026-07-07] :: 🛡️ :: Split failure tracking into `failures` (all types → backoff) and `crashFailures` (subprocess crashes only → maxFailures safety net); waitCmd classifies the captured line via classifyOutput — definitive rejection (403/auth/forbidden/room/provider) clears the provider and removes last_provider.json immediately instead of waiting ~23 min; maxFailures block moved fully under the mutex (fixes data race); added ClearProvider() for /n stop (clear + remove file + signal); host-side start faults bump `failures` only so they can never trigger file removal
[2026-07-07] :: 🚀 :: Added SetRoute method; route promotion to "proxy" on stable>30s; demotion to "direct" on crash; uses singbox.Controller interface
[2026-07-06] :: 🐛 :: Capture subprocess stdout/stderr (os.Pipe + streamSubprocess); surface the real olcRTC error (e.g. wbstream 403 "guests cannot create rooms") in logs and Status.LastError instead of discarding it as "exit status 1"
[2026-07-06] :: 🛡️ :: Fixed DDoS restart loop: added exponential backoff (calcBackoff), crash detection in waitCmd (sets lastError + increments failures), interruptible backoff sleep — prevents 300ms hammering of upstream on repeated 429 failures
[2026-07-02] :: 🚀 :: Added StatusText() method and snapshot() extraction for human-readable Russian-emoji status rendering
[2026-07-02] :: 🛡️ :: Restructured loop: mutex released before select — SetProvider/waitCmd no longer blocked for up to 60s
[2026-07-02] :: 🐛 :: Fixed nil pointer panic in loop: goroutine captured c.cmd by closure, raced against c.cmd=nil after Wait()
*/

// Package controller manages the olcrtc subprocess lifecycle.
package controller

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"dial-up/internal/config"
	"dial-up/internal/domain/logger"
	"dial-up/internal/firewall"
	"dial-up/internal/provider"
	"dial-up/internal/singbox"
)

const maxFailures = 5

// Status is an immutable snapshot of controller state for status reporting.
type Status struct {
	HasProcess     bool               `json:"has_process"`
	Provider       *provider.Provider `json:"provider"`
	ProcessStarted *time.Time         `json:"process_started"`
	ProcessStopped *time.Time         `json:"process_stopped"`
	LastExitCode   *int               `json:"last_exit_code"`
	LastError      string             `json:"last_error"`
	Restarting     bool               `json:"restarting"`
	Failures       int                `json:"failures"`
	CrashFailures  int                `json:"crash_failures"`
	PingDNS        string             `json:"ping_dns"`
	SingBoxAlive   *bool              `json:"sing_box_alive"`
	SingBoxRoute   string             `json:"sing_box_route"`
}

// Controller manages the olcrtc subprocess lifecycle in a single goroutine loop.
type Controller struct {
	mu  sync.Mutex
	cfg *config.Config
	l   logger.Logger

	singbox  singbox.Controller
	netCache *netCache
	guardian *tproxyGuardian

	cmd           *exec.Cmd
	provider      *provider.Provider
	restarting    bool
	savePending   bool
	failures      int
	crashFailures int
	startedAt     *time.Time
	stoppedAt     *time.Time
	lastExitCode  *int
	lastError     string
	lastOutput    string
	streamWg      *sync.WaitGroup

	done chan struct{}
	sig  chan struct{}
}

// New creates a Controller with the given config and logger.
func New(cfg *config.Config, l logger.Logger, singbox singbox.Controller) *Controller {
	c := &Controller{
		cfg:      cfg,
		l:        l,
		singbox:  singbox,
		netCache: newNetCache(singbox, cfg.IsClient, l),

		done: make(chan struct{}),
		sig:  make(chan struct{}, 1),
	}
	// The tproxy health guardian is a client-only concern: only the OpenWrt router
	// runs the nft tproxy redirect and sing-box. It refreshes netCache on demotion.
	if cfg.IsClient {
		c.guardian = newTproxyGuardian(singbox, firewall.New(l), l, c.netCache.refresh)
	}
	return c
}

// - ctx: Cancellation context for shutdown

// Start loads the last provider and starts the controller loop goroutine.
func (c *Controller) Start(ctx context.Context) {
	cl := c.l.With(logger.Function("Controller.Start"))

	p, err := LoadLastProvider(c.cfg.LastProviderFile)
	if err == nil {
		cl.Info("controller", "Loaded last provider", logger.Block("LoadLastProvider"), logger.Status("OK"), logger.Importance(5), logger.String("provider", p.Kind), logger.String("room", p.RoomID))
		c.SetProvider(&p, false)
	} else {
		cl.Debug("controller", "No last provider to load", logger.Block("LoadLastProvider"), logger.Status("SKIP"), logger.Importance(3))
	}

	c.netCache.Start(ctx)

	if c.guardian != nil {
		c.guardian.Start(ctx)
	}

	go c.loop(ctx)
	cl.Info("controller", "Controller loop started", logger.Status("OK"), logger.Importance(5))
}

// Stop terminates the controller loop, stops the diagnostics cache, and kills the child process.
func (c *Controller) Stop() {
	cl := c.l.With(logger.Function("Controller.Stop"))

	close(c.done)
	c.netCache.Stop()

	if c.guardian != nil {
		c.guardian.Stop()
	}

	c.mu.Lock()
	if c.cmd != nil && c.cmd.Process != nil {
		cl.Info("controller", "Killing subprocess", logger.Block("TerminateProcess"), logger.Status("ATTEMPT"), logger.Importance(7))
		_ = c.cmd.Process.Kill()
	}
	c.mu.Unlock()

	cl.Info("controller", "Controller stopped", logger.Status("OK"), logger.Importance(5))
}

// SetProvider sets the active provider and triggers a restart signal.
func (c *Controller) SetProvider(p *provider.Provider, save bool) {
	c.mu.Lock()
	if p == nil && c.provider == nil {
		c.mu.Unlock()
		return
	}
	if p != nil && c.provider != nil && p.Kind == c.provider.Kind && p.RoomID == c.provider.RoomID {
		c.mu.Unlock()
		return
	}
	c.provider = p
	c.restarting = true
	c.failures = 0
	c.crashFailures = 0
	c.lastError = ""
	if save {
		c.savePending = true
	}
	c.mu.Unlock()

	select {
	case c.sig <- struct{}{}:
	default:
	}
}

// ClearProvider stops the provider and deletes the persisted last_provider.json.
func (c *Controller) ClearProvider() {
	cl := c.l.With(logger.Function("Controller.ClearProvider"))

	cl.Info("controller", "Clearing provider and resetting counters", logger.Block("ClearState"), logger.Status("ATTEMPT"), logger.Importance(7))

	c.mu.Lock()
	c.provider = nil
	c.restarting = true
	c.failures = 0
	c.crashFailures = 0
	c.savePending = false
	c.lastError = ""
	c.mu.Unlock()

	cl.Info("controller", "Provider cleared", logger.Block("ClearState"), logger.Status("OK"), logger.Importance(7))

	RemoveLastProvider(c.cfg.LastProviderFile, c.l)
	cl.Info("controller", "Removed persisted provider", logger.Block("ForgetProvider"), logger.Status("OK"), logger.Importance(7))

	select {
	case c.sig <- struct{}{}:
	default:
	}
}

// Restart triggers a restart of the current subprocess.
func (c *Controller) Restart() {
	c.mu.Lock()
	c.restarting = true
	c.failures = 0
	c.lastError = ""
	c.mu.Unlock()

	select {
	case c.sig <- struct{}{}:
	default:
	}
}

// SetRoute manually switches the sing-box selector route (used by /m command).
// No-op when IsClient is false (server mode does not use sing-box).
// After a successful switch, triggers a cache refresh so /s shows the new route promptly.
func (c *Controller) SetRoute(mode string) error {
	if !c.cfg.IsClient {
		return nil
	}
	if err := c.singbox.SetRoute(mode); err != nil {
		return fmt.Errorf("singbox: %w", err)
	}
	go c.netCache.refresh()
	return nil
}

// calcBackoff returns an exponential backoff duration for repeated failures.
// Must be called while holding c.mu. With SleepOnError=5: 5s, 10s, 20s, 40s, 80s, 160s, 300s(cap).
func (c *Controller) calcBackoff() time.Duration {
	if c.failures == 0 {
		return 0
	}
	base := time.Duration(c.cfg.SleepOnError) * time.Second
	if base <= 0 {
		base = 5 * time.Second
	}
	d := base * time.Duration(1<<uint(c.failures-1))
	if d > 5*time.Minute || d <= 0 {
		d = 5 * time.Minute
	}
	return d
}

func (c *Controller) snapshot() Status {
	c.mu.Lock()
	s := Status{
		HasProcess:     c.cmd != nil,
		ProcessStarted: c.startedAt,
		ProcessStopped: c.stoppedAt,
		LastExitCode:   c.lastExitCode,
		LastError:      c.lastError,
		Restarting:     c.restarting,
		Failures:       c.failures,
		CrashFailures:  c.crashFailures,
	}
	if c.provider != nil {
		s.Provider = new(*c.provider)
	}
	c.mu.Unlock()

	ns := c.netCache.get()
	s.PingDNS = ns.pingDNS

	if c.cfg.IsClient && ns.hasSingBox {
		alive := ns.sbAlive
		s.SingBoxAlive = &alive
		s.SingBoxRoute = ns.sbRoute
	}

	return s
}

// Status returns a snapshot of the current controller state.
func (c *Controller) Status() Status {
	return c.snapshot()
}

// StatusText returns a human-readable Russian-emoji formatted status string.
func (c *Controller) StatusText() string {
	return RenderStatus(c.snapshot())
}

func (c *Controller) loop(ctx context.Context) {
	cl := c.l.With(logger.Function("Controller.loop"))

	const idleTimeout = 60 * time.Second

	for {
		func() {
			// Recover defers run LIFO. The inner defer (conditional unlock) runs BEFORE the
			// outer defer (recover). This ensures mutex is unlocked before recover tries to lock it,
			// avoiding deadlock on non-reentrant sync.Mutex when panic occurs in StateEval.
			defer func() {
				if r := recover(); r != nil {
					cl.Error("controller", "Loop panic recovered", logger.Block("RecoverGuard"), logger.Status("FAIL"), logger.Importance(10), logger.Any("panic", r))
					c.mu.Lock()
					c.lastError = fmt.Sprintf("panic: %v", r)
					c.mu.Unlock()
				}
			}()

			locked := true
			defer func() {
				if locked {
					c.mu.Unlock()
				}
			}()

			// Mutex previously held ACROSS the select (lines 292-299 old), blocking SetProvider/waitCmd
			// for up to 60s. Now mutex is released BEFORE select — SetProvider and waitCmd can signal freely.
			c.mu.Lock()

			hasProcess := c.cmd != nil
			wait := idleTimeout

			if hasProcess {
				if c.restarting {
					cl.Info("controller", "Restarting: terminating process", logger.Block("LoopTick"), logger.Status("ATTEMPT"), logger.Importance(6))
					if c.cmd.Process != nil {
						_ = c.cmd.Process.Signal(syscall.SIGTERM)
						proc := c.cmd.Process
						go func() {
							time.Sleep(time.Second)
							proc.Kill()
						}()
					}
					// waitCmd goroutine for the same cmd also calls cmd.Wait(). The loop's Wait()
					// blocks until process exits, ensuring c.cmd=nil is set before we release the lock.
					// waitCmd's own Wait() returns immediately after (Go Process.Wait is mutex-guarded)
					// and skips its state update because c.cmd != cmd.
					_ = c.cmd.Wait()
					c.cmd = nil
					wait = time.Second
				} else {
					wait = 10 * time.Second
					// Process survived >30s — consider it healthy
					if c.startedAt != nil && time.Since(*c.startedAt) > 30*time.Second {
						// Reset backoff counters only if there were prior failures
						if c.failures > 0 {
							cl.Info("controller", "Process stable, resetting backoff", logger.Block("LoopTick"), logger.Status("OK"), logger.Importance(4), logger.Any("uptime", time.Since(*c.startedAt).String()))
							c.failures = 0
							c.crashFailures = 0
							c.lastError = ""
						}

						// Previously gated by c.failures > 0, so promotion never fired on
						// first successful start (failures == 0). Now promotes unconditionally on >30s
						// stability so the route is never stuck at "direct" on a healthy first run.
						if c.cfg.IsClient {
							cl.Info("controller", "Promoting to proxy route", logger.Block("LoopTick"), logger.Status("ATTEMPT"), logger.Importance(6))
							if err := c.singbox.SetRoute(singbox.ModeProxy); err != nil {
								cl.Warn("controller", "Failed to set proxy route, will retry", logger.Block("LoopTick"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
							}
						}
					}
				}
			} else {
				c.restarting = false
				if c.provider != nil {
					cl.Info("controller", "Starting new process", logger.Block("LoopTick"), logger.Status("ATTEMPT"), logger.Importance(6))
					configContent := RenderConfig(RenderParams{
						IsClient:       c.cfg.IsClient,
						Provider:       c.provider.Kind,
						RoomID:         c.provider.RoomID,
						OlcrtcKey:      c.cfg.OlcrtcKey,
						SocksProxyAddr: c.cfg.SocksProxyAddr,
						SocksProxyPort: c.cfg.SocksProxyPort,
						SocksProxyUser: c.cfg.SocksProxyUser,
						SocksProxyPass: c.cfg.SocksProxyPass,
					})
					configFile := "cnc.yaml"
					if !c.cfg.IsClient {
						configFile = "srv.yaml"
					}
					if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
						cl.Error("controller", "Failed to write config", logger.Block("LoopTick"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err))
						c.lastError = err.Error()
						c.failures++
						locked = false
						c.mu.Unlock()
						return
					}
					cmd := exec.Command(c.cfg.OlcrtcExe, configFile)
					// surface in logs and /s instead of being discarded. os.Pipe gives the child a real fd.
					pr, pw, err := os.Pipe()
					if err != nil {
						cl.Error("controller", "Failed to create subprocess pipe", logger.Block("LoopTick"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err))
						c.lastError = err.Error()
						c.failures++
						locked = false
						c.mu.Unlock()
						return
					}
					cmd.Stdout = pw
					cmd.Stderr = pw
					if err := cmd.Start(); err != nil {
						_ = pr.Close()
						_ = pw.Close()
						cl.Error("controller", "Failed to start subprocess", logger.Block("LoopTick"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err))
						c.lastError = err.Error()
						c.failures++
						locked = false
						c.mu.Unlock()
						return
					}
					// Parent no longer needs the write end; the child owns its own copy of the fd.
					_ = pw.Close()
					wg := &sync.WaitGroup{}
					wg.Add(1)
					c.cmd = cmd
					c.streamWg = wg
					c.lastOutput = ""
					c.startedAt = new(time.Now())
					c.stoppedAt = nil
					c.lastExitCode = nil
					c.lastError = ""
					go c.streamSubprocess(pr, wg)
					go c.waitCmd(cmd)
					wait = 3 * time.Second
				}
				if c.savePending {
					c.savePending = false
					if c.provider != nil {
						SaveLastProvider(c.cfg.LastProviderFile, *c.provider, c.l)
					}
				}
			}

			// too many times (crashFailures), not on host-side start faults (which only bump `failures`
			// for backoff). Fully under the mutex.
			// Previously this block ran AFTER c.mu.Unlock(), writing c.failures/c.provider/
			// c.lastError without the mutex (data race detected under -race). Now it is evaluated inside
			// the locked section; the actual file removal is deferred to after unlock (best-effort fs op).
			removeOnMax := false
			if c.crashFailures > maxFailures {
				cl.Warn("controller", "Too many subprocess crashes, removing provider", logger.Block("maxFailures"), logger.Status("FAIL"), logger.Importance(8), logger.Int("crashFailures", c.crashFailures), logger.Int("failures", c.failures))
				c.provider = nil
				c.lastError = "provider is corrupted, persistence and runtime removed"
				c.failures = 0
				c.crashFailures = 0
				removeOnMax = true
			}

			locked = false
			c.mu.Unlock()

			if removeOnMax {
				RemoveLastProvider(c.cfg.LastProviderFile, c.l)
			}

			select {
			case <-c.done:
				return
			case <-ctx.Done():
				return
			case <-c.sig:
			case <-time.After(wait):
			}
		}()

		select {
		case <-c.done:
			cl.Info("controller", "Loop exiting via done", logger.Block("LoopBody"), logger.Status("OK"), logger.Importance(5))
			return
		case <-ctx.Done():
			cl.Info("controller", "Loop exiting via ctx", logger.Block("LoopBody"), logger.Status("OK"), logger.Importance(5))
			return
		default:
			// Old code used time.Sleep (blocking, not interruptible by ctx/done) and only fired when lastError!="".
			// waitCmd never set lastError on crash → no backoff → 300ms restart loop. Now uses failures counter + calcBackoff.
			// c.failures was read for the log AFTER mu.Unlock (data race with waitCmd/SetProvider/ClearProvider);
			// now snapshot under the same lock as calcBackoff.
			c.mu.Lock()
			backoff, fails := c.calcBackoff(), c.failures
			c.mu.Unlock()
			if backoff > 0 {
				cl.Warn("controller", "Backing off after failure", logger.Block("Backoff"), logger.Status("SKIP"), logger.Importance(6), logger.Any("backoff", backoff.String()), logger.Any("failures", fails))
				select {
				case <-time.After(backoff):
				case <-c.done:
					return
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (c *Controller) waitCmd(cmd *exec.Cmd) {
	cl := c.l.With(logger.Function("Controller.waitCmd"))

	err := cmd.Wait()

	// Snapshot the owning-cmd flag and the per-process stream WaitGroup under lock, then release
	// the lock before waiting so streamSubprocess can flush lastOutput without being blocked.
	c.mu.Lock()
	owns := c.cmd == cmd
	wg := c.streamWg
	c.mu.Unlock()

	if owns && wg != nil {
		// Child exited → its pipe write end is closed → the streamer drains remaining lines and
		// calls Done(). Bound the wait so a stuck pipe cannot freeze the loop.
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cl.Warn("controller", "Timed out waiting for subprocess output stream", logger.Block("ExitEval"), logger.Status("SKIP"), logger.Importance(6))
		}
	}

	removeFile := false
	c.mu.Lock()
	if c.cmd == cmd {
		c.cmd = nil
		c.streamWg = nil
		c.stoppedAt = new(time.Now())
		code := 0
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			code = exitErr.ExitCode()
		}
		c.lastExitCode = new(code)
		if code != 0 {
			// Previously lastError was set to "exit status 1" (useless) and subprocess stderr
			// was discarded — operators could not see the real cause (e.g. "guests cannot create rooms").
			// Now prefer the last captured subprocess line; fall back only if nothing was captured.
			msg := strings.TrimSpace(c.lastOutput)
			if msg == "" {
				if err != nil {
					msg = err.Error()
				} else {
					msg = fmt.Sprintf("process exited with code %d", code)
				}
			}

			class := ClassifyOutput(msg)
			if class == ClassDefinitive {
				// Definitive tunnel rejection — provider data is permanently unusable. Drop it now
				// instead of running the ~23 min maxFailures cascade against a guaranteed-to-fail upstream.
				c.provider = nil
				c.failures = 0
				c.crashFailures = 0
				c.lastError = fmt.Sprintf("⛔ Провайдер отклонён тоннелем (%s), сохранение удалено", msg)
				removeFile = true
				cl.Error("controller", "Definitive provider rejection, removing persistence", logger.Block("ExitEval"), logger.Status("FAIL"), logger.Importance(9), logger.Any("exit_code", code), logger.String("captured", msg), logger.String("class", "definitive"))
			} else {
				// Transient failure — retry with backoff; count toward the maxFailures safety net.
				c.failures++
				c.crashFailures++
				c.lastError = msg
				cl.Warn("controller", "Subprocess crashed", logger.Block("ExitEval"), logger.Status("FAIL"), logger.Importance(8), logger.Any("exit_code", code), logger.Int("failures", c.failures), logger.Int("crashFailures", c.crashFailures), logger.String("captured", msg), logger.String("class", "transient"))

				// Best-effort demote to direct route so LAN keeps internet access when olcrtc is down.
				if c.cfg.IsClient {
					if err := c.singbox.SetRoute(singbox.ModeDirect); err != nil {
						cl.Warn("controller", "Failed to set direct route after crash", logger.Block("ExitEval"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
					}
				}
			}
		} else {
			cl.Info("controller", "Subprocess exited cleanly", logger.Block("ExitEval"), logger.Status("OK"), logger.Importance(5))
		}
	}
	c.mu.Unlock()

	if removeFile {
		RemoveLastProvider(c.cfg.LastProviderFile, c.l)
	}

	select {
	case c.sig <- struct{}{}:
	default:
	}
}

// streamSubprocess drains the subprocess pipe, logging each line and retaining the last non-empty one.
func (c *Controller) streamSubprocess(pr *os.File, wg *sync.WaitGroup) {
	cl := c.l.With(logger.Function("Controller.streamSubprocess"))
	defer wg.Done()
	defer pr.Close()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		cl.Info("controller", "olcRTC: "+line, logger.Block("Subprocess"), logger.Status("OK"), logger.Importance(4), logger.String("line", line))
		c.mu.Lock()
		c.lastOutput = line
		c.mu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		cl.Warn("controller", "Subprocess pipe read error", logger.Block("Subprocess"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
	}
}
