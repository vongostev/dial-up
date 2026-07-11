/*
[2026-07-10] :: 🚀 :: Initial firewall package: Manager interface + ExecManager with injectable runner for nft flush / fw4 reload / nft list
*/

// Package firewall provides a thin abstraction over OpenWrt nft/fw4 shell commands.
package firewall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"dial-up/internal/domain/logger"
)

const logCategory = "firewall"

// nft family/table/chain identifying the sing-box tproxy redirect.
const (
	nftFamily = "inet"
	nftTable  = "fw4"
	nftChain  = "singbox_tproxy"
)

// Sentinel errors wrapping firewall command failures. Each is wrapped around the
// underlying exec error so callers can errors.Is against the firewall layer.

var (
	// ErrNftFlush wraps a failed `nft flush chain` invocation.
	ErrNftFlush = errors.New("nft flush chain failed")
	// ErrFw4Reload wraps a failed `fw4 reload` invocation.
	ErrFw4Reload = errors.New("fw4 reload failed")
	// ErrNftList wraps a failed `nft list chain` invocation.
	ErrNftList = errors.New("nft list chain failed")
)

// RunnerFunc executes a command and returns its combined stdout/stderr output.
type RunnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Manager defines the firewall operations consumed by the tproxy health guardian.
type Manager interface {
	FlushTproxy(ctx context.Context) error
	ReloadFw4(ctx context.Context) error
	TproxyRulesPresent(ctx context.Context) (bool, error)
}

// ExecManager implements Manager by shelling out to nft/fw4 via an injectable runner.
type ExecManager struct {
	runner RunnerFunc
	l      logger.Logger
}

// execRunner runs a command via exec.CommandContext and returns combined stdout+stderr.
//
//nolint:gosec,wrapcheck // G204: name/args are injectable for testing; the production commands are fixed internal constants (nft/fw4). wrapcheck: this is the intentional exec boundary — callers (FlushTproxy/ReloadFw4/TproxyRulesPresent) wrap the returned error with domain sentinels (ErrNftFlush etc.).
func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// New creates an ExecManager backed by exec.CommandContext for real nft/fw4 calls.
func New(l logger.Logger) *ExecManager {
	return &ExecManager{runner: execRunner, l: l}
}

// NewWithRunner creates an ExecManager backed by the given runner function.
func NewWithRunner(l logger.Logger, runner RunnerFunc) *ExecManager {
	return &ExecManager{runner: runner, l: l}
}

// FlushTproxy empties the singbox_tproxy chain, stopping LAN traffic redirection.
func (m *ExecManager) FlushTproxy(ctx context.Context) error {
	cl := m.l.With(logger.Function("ExecManager.FlushTproxy"))

	cl.Debug(logCategory, "Flushing tproxy chain", logger.Block("NftFlush"), logger.Status("ATTEMPT"), logger.Importance(6))

	out, err := m.runner(ctx, "nft", "flush", "chain", nftFamily, nftTable, nftChain)
	if err != nil {
		cl.Error(logCategory, "nft flush chain failed", logger.Block("NftFlush"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err), logger.String("output", string(out)))
		return fmt.Errorf("%w: %s: %w", ErrNftFlush, strings.TrimSpace(string(out)), err)
	}

	cl.Info(logCategory, "Tproxy chain flushed", logger.Block("NftFlush"), logger.Status("OK"), logger.Importance(7))

	return nil
}

// ReloadFw4 rebuilds the nft ruleset, restoring the tproxy chain.
func (m *ExecManager) ReloadFw4(ctx context.Context) error {
	cl := m.l.With(logger.Function("ExecManager.ReloadFw4"))

	cl.Debug(logCategory, "Reloading fw4 ruleset", logger.Block("Fw4Reload"), logger.Status("ATTEMPT"), logger.Importance(6))

	out, err := m.runner(ctx, "fw4", "reload")
	if err != nil {
		cl.Error(logCategory, "fw4 reload failed", logger.Block("Fw4Reload"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err), logger.String("output", string(out)))
		return fmt.Errorf("%w: %s: %w", ErrFw4Reload, strings.TrimSpace(string(out)), err)
	}

	cl.Info(logCategory, "fw4 reloaded", logger.Block("Fw4Reload"), logger.Status("OK"), logger.Importance(7))

	return nil
}

// TproxyRulesPresent reports whether the tproxy chain currently has redirect rules.
func (m *ExecManager) TproxyRulesPresent(ctx context.Context) (bool, error) {
	cl := m.l.With(logger.Function("ExecManager.TproxyRulesPresent"))

	cl.Debug(logCategory, "Listing tproxy chain", logger.Block("NftList"), logger.Status("ATTEMPT"), logger.Importance(5))

	out, err := m.runner(ctx, "nft", "list", "chain", nftFamily, nftTable, nftChain)
	if err != nil {
		cl.Error(logCategory, "nft list chain failed", logger.Block("NftList"), logger.Status("FAIL"), logger.Importance(7), logger.Error(err), logger.String("output", string(out)))
		return false, fmt.Errorf("%w: %s: %w", ErrNftList, strings.TrimSpace(string(out)), err)
	}

	present := strings.Contains(string(out), "tproxy")
	cl.Info(logCategory, "Tproxy presence detected", logger.Block("NftList"), logger.Status("OK"), logger.Importance(5), logger.Bool("present", present))

	return present, nil
}
