/*
[2026-07-10] :: 🚀 :: Initial firewall ExecManager test suite: command construction + TproxyRulesPresent parsing + sentinel wrapping
*/

package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"dial-up/internal/domain/logger"
	"dial-up/internal/firewall"
)

// errExecFake is a static sentinel simulating a non-zero exec exit for the fake runner.
var errExecFake = errors.New("exit status 1")

func TestExecManagerFlushTproxyCommand(t *testing.T) {
	var gotName string
	var gotArgs []string
	runner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = args
		return nil, nil
	}

	m := firewall.NewWithRunner(logger.New(true), runner)
	if err := m.FlushTproxy(context.Background()); err != nil {
		t.Fatalf("FlushTproxy returned error: %v", err)
	}

	wantName := "nft"
	wantArgs := []string{"flush", "chain", "inet", "fw4", "singbox_tproxy"}

	if gotName != wantName {
		t.Errorf("command name = %q, want %q", gotName, wantName)
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("command args = %v, want %v", gotArgs, wantArgs)
	}
	t.Logf("FlushTproxy ran: %s %v", gotName, gotArgs)
}

func TestExecManagerReloadFw4Command(t *testing.T) {
	var gotName string
	var gotArgs []string
	runner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = args
		return nil, nil
	}

	m := firewall.NewWithRunner(logger.New(true), runner)
	if err := m.ReloadFw4(context.Background()); err != nil {
		t.Fatalf("ReloadFw4 returned error: %v", err)
	}

	wantName := "fw4"
	wantArgs := []string{"reload"}

	if gotName != wantName {
		t.Errorf("command name = %q, want %q", gotName, wantName)
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("command args = %v, want %v", gotArgs, wantArgs)
	}
	t.Logf("ReloadFw4 ran: %s %v", gotName, gotArgs)
}

func TestExecManagerTproxyRulesPresent(t *testing.T) {
	tests := []struct {
		name   string
		output []byte
		want   bool
	}{
		{
			name:   "rules present (output has tproxy redirect)",
			output: []byte("chain singbox_tproxy {\n  iifname \"br-lan\" counter tproxy ip to 127.0.0.1:2080 accept\n}"),
			want:   true,
		},
		{
			name:   "rules absent (output has no tproxy statement)",
			output: []byte("chain something_else {\n  type filter hook prerouting priority mangle; policy accept;\n}"),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return tt.output, nil
			}
			m := firewall.NewWithRunner(logger.New(true), runner)

			got, err := m.TproxyRulesPresent(context.Background())
			if err != nil {
				t.Fatalf("TproxyRulesPresent returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("present = %v, want %v", got, tt.want)
			}
			t.Logf("TproxyRulesPresent: output=%q → present=%v", tt.output, got)
		})
	}
}

func TestExecManagerFlushTproxyErrorWrap(t *testing.T) {
	runner := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("device or resource busy"), errExecFake
	}

	m := firewall.NewWithRunner(logger.New(true), runner)
	err := m.FlushTproxy(context.Background())

	if err == nil {
		t.Fatal("expected error from failing runner, got nil")
	}
	if !errors.Is(err, firewall.ErrNftFlush) {
		t.Errorf("expected error to wrap ErrNftFlush, got: %v", err)
	}
	t.Logf("wrapped error: %v", err)
}
