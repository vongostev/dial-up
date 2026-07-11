/*
[2026-07-08] :: 🔌 :: Updated fakeSingBox to satisfy new singbox.Controller interface (added Status() method)
[2026-07-07] :: 🚀 :: Added classified-removal integration tests: definitive crash removes file on 1st crash; transient crash keeps file + bumps crashFailures; env start-fault keeps file + leaves crashFailures at 0; ClearProvider removes file
[2026-07-06] :: 🚀 :: Added TestControllerCapturesSubprocessError verifying subprocess stderr is captured into Status.LastError on crash
[2026-07-02] :: 🚀 :: Added RenderStatus and StatusText test coverage
*/

package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dial-up/internal/config"
	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
	"dial-up/internal/provider"
	"dial-up/internal/singbox"
)

// fakeSingBox implements singbox.Controller for tests.
type fakeSingBox struct{}

func (f *fakeSingBox) SetRoute(_ string) error         { return nil }
func (f *fakeSingBox) Status() (singbox.Status, error) { return singbox.Status{}, nil }

func TestControllerSetProviderNil(t *testing.T) {
	l := logger.New(true)
	ctrl := controller.New(&config.Config{IsClient: true, SleepOnError: 1}, l, &fakeSingBox{})

	// SetProvider(nil) when provider is already nil should be a no-op
	ctrl.SetProvider(nil, false)

	s := ctrl.Status()
	if s.Provider != nil {
		t.Error("expected nil provider after SetProvider(nil)")
	}
}

func TestControllerSetProviderSame(_ *testing.T) {
	l := logger.New(true)
	ctrl := controller.New(&config.Config{IsClient: true, SleepOnError: 1}, l, &fakeSingBox{})

	p := &provider.Provider{Kind: "wbstream", RoomID: "abc"}
	ctrl.SetProvider(p, false)

	// Same provider should be a no-op (no restart triggered)
	ctrl.SetProvider(p, false)
}

func TestControllerStartStop(_ *testing.T) {
	l := logger.New(true)
	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	ctx, cancel := context.WithCancel(context.Background())
	ctrl.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()
	ctrl.Stop()
}

func TestControllerStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last_provider.json")

	// Save
	controller.SaveLastProvider(path, provider.Provider{Kind: "wbstream", RoomID: "abc-123"}, logger.New(true))

	// Load back
	p, err := controller.LoadLastProvider(path)
	if err != nil {
		t.Fatalf("LoadLastProvider failed: %v", err)
	}
	if p.Kind != "wbstream" || p.RoomID != "abc-123" {
		t.Errorf("got %s/%s, want wbstream/abc-123", p.Kind, p.RoomID)
	}
}

func TestControllerStateArrayFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last_provider.json")

	controller.SaveLastProvider(path, provider.Provider{Kind: provider.ProviderTelemost, RoomID: "42"}, logger.New(true))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	// Verify JSON array format, not object
	if string(data) != `["telemost","42"]` {
		t.Errorf("expected JSON array, got: %s", string(data))
	}
}

func TestControllerStatusFields(t *testing.T) {
	l := logger.New(true)
	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	p := &provider.Provider{Kind: "wbstream", RoomID: "abc"}
	ctrl.SetProvider(p, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)

	time.Sleep(200 * time.Millisecond)

	s := ctrl.Status()
	if s.Provider == nil || s.Provider.Kind != "wbstream" {
		t.Error("expected provider kind wbstream in status")
	}
}

func TestRenderStatus(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name   string
		status controller.Status
		checks []string // substrings that must appear
		anti   []string // substrings that must NOT appear
	}{
		{
			name:   "empty status",
			status: controller.Status{},
			checks: []string{
				"🤖 Статус",
				"━━━━━━━━━━━━━━━━",
				"🔴 Процесс: остановлен",
				"📦 Провайдер: не задан",
				"🕒 Запущен: —",
				"🛑 Остановлен: —",
				"🔢 Код выхода: —",
				"🔁 Перезапуск: нет",
				"⚠️ Ошибка: нет",
				"🌐 Пинг DNS (9.9.9.9): —",
			},
			anti: []string{"📦 Sing-Box:", "🔀 Маршрут:"},
		},
		{
			name: "running with provider and errors",
			status: controller.Status{
				HasProcess:     true,
				Provider:       &provider.Provider{Kind: "wbstream", RoomID: "abc-123"},
				ProcessStarted: &now,
				ProcessStopped: nil,
				LastExitCode:   new(0),
				LastError:      "something went wrong",
				Restarting:     true,
			},
			checks: []string{
				"🤖 Статус",
				"━━━━━━━━━━━━━━━━",
				"🟢 Процесс: работает",
				"📦 Провайдер: wbstream · abc-123",
				"🕒 Запущен: " + now.Local().Format("2006-01-02 15:04:05"),
				"🛑 Остановлен: —",
				"🔢 Код выхода: 0",
				"🔁 Перезапуск: да",
				"⚠️ Ошибка: something went wrong",
			},
			anti: []string{"не задан", "остановлен", "нет"},
		},
		{
			name: "stopped with exit code and stopped time",
			status: controller.Status{
				HasProcess:     false,
				Provider:       &provider.Provider{Kind: provider.ProviderTelemost, RoomID: "42"},
				ProcessStarted: &now,
				ProcessStopped: &now,
				LastExitCode:   new(1),
				LastError:      "",
				Restarting:     false,
				PingDNS:        "15ms",
			},
			checks: []string{
				"🔴 Процесс: остановлен",
				"📦 Провайдер: telemost · 42",
				"🕒 Запущен: " + now.Local().Format("2006-01-02 15:04:05"),
				"🛑 Остановлен: " + now.Local().Format("2006-01-02 15:04:05"),
				"🔢 Код выхода: 1",
				"🔁 Перезапуск: нет",
				"⚠️ Ошибка: нет",
				"🌐 Пинг DNS (9.9.9.9): 15ms",
			},
			anti: []string{"🟢", "работает", "не задан"},
		},
		{
			name: "client with sing-box alive and proxy route",
			status: func() controller.Status {
				alive := true
				return controller.Status{
					HasProcess:   true,
					PingDNS:      "8ms",
					SingBoxAlive: &alive,
					SingBoxRoute: singbox.ModeProxy,
				}
			}(),
			checks: []string{
				"🌐 Пинг DNS (9.9.9.9): 8ms",
				"📦 Sing-Box: 🟢 работает",
				"🔀 Маршрут: proxy",
			},
		},
		{
			name: "client with sing-box dead",
			status: func() controller.Status {
				dead := false
				return controller.Status{
					PingDNS:      "timeout",
					SingBoxAlive: &dead,
					SingBoxRoute: "",
				}
			}(),
			checks: []string{
				"🌐 Пинг DNS (9.9.9.9): timeout",
				"📦 Sing-Box: 🔴 не отвечает",
				"🔀 Маршрут: —",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.RenderStatus(tt.status)
			for _, check := range tt.checks {
				if !strings.Contains(result, check) {
					t.Errorf("expected result to contain %q\nresult:\n%s", check, result)
				}
			}
			for _, anti := range tt.anti {
				if strings.Contains(result, anti) {
					t.Errorf("expected result NOT to contain %q\nresult:\n%s", anti, result)
				}
			}
		})
	}
}

func TestControllerStatusText(t *testing.T) {
	l := logger.New(true)
	ctrl := controller.New(&config.Config{IsClient: true, SleepOnError: 1}, l, &fakeSingBox{})

	text := ctrl.StatusText()
	if !strings.Contains(text, "🤖 Статус") {
		t.Errorf("StatusText should contain status header, got:\n%s", text)
	}
	if !strings.Contains(text, "🔴 Процесс: остановлен") {
		t.Errorf("StatusText should indicate stopped process, got:\n%s", text)
	}
}

func TestControllerCapturesSubprocessError(t *testing.T) {
	// START_BLOCK_SETUP
	prevDir, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	// Fake olcrtc: writes the real failure cause to stderr, then exits 1.
	script := filepath.Join(dir, "fakeolcrtc.sh")
	body := "#!/bin/sh\necho 'carrier auth failed: status 403 guests cannot create rooms' >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	l := logger.New(true)
	cfg := &config.Config{IsClient: true, SleepOnError: 1, OlcrtcExe: script, LastProviderFile: "last_provider.json"}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// END_BLOCK_SETUP

	// START_BLOCK_RUN
	ctrl.Start(ctx)
	ctrl.SetProvider(&provider.Provider{Kind: "wbstream", RoomID: "room-1"}, false)

	// Poll for the crash window: process attempted, exited, lastError populated.
	deadline := time.Now().Add(8 * time.Second)
	var s controller.Status
	for time.Now().Before(deadline) {
		s = ctrl.Status()
		if !s.HasProcess && s.LastExitCode != nil && s.LastError != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// END_BLOCK_RUN

	// START_BLOCK_ASSERT
	if s.LastExitCode == nil || *s.LastExitCode != 1 {
		t.Fatalf("expected last exit code 1, got: %+v", s.LastExitCode)
	}
	if !strings.Contains(s.LastError, "guests cannot create rooms") {
		t.Fatalf("expected LastError to contain captured stderr line, got: %q", s.LastError)
	}
	// END_BLOCK_ASSERT
}

// writeFakeOlcrtc writes a shell script that emulates olcrtc and returns its path.
func writeFakeOlcrtc(t *testing.T, dir, body string) string {
	t.Helper()
	script := filepath.Join(dir, "fakeolcrtc.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake olcrtc: %v", err)
	}
	return script
}

// chdirTemp chdirs to a fresh temp dir for the test and restores cwd on cleanup.
func chdirTemp(t *testing.T) string {
	t.Helper()
	prevDir, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })
	return dir
}

func TestControllerRemovesProviderOnDefinitiveCrash(t *testing.T) {
	dir := chdirTemp(t)
	script := writeFakeOlcrtc(t, dir, "echo 'carrier auth failed: status 403 guests cannot create rooms' >&2\nexit 1\n")

	l := logger.New(true)
	cfg := &config.Config{IsClient: true, SleepOnError: 1, OlcrtcExe: script, LastProviderFile: "last_provider.json"}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	// Pre-create the persisted file so "removed" is a meaningful observation.
	controller.SaveLastProvider("last_provider.json", provider.Provider{Kind: "wbstream", RoomID: "room-1"}, l)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)
	ctrl.SetProvider(&provider.Provider{Kind: "wbstream", RoomID: "room-1"}, true)

	// Poll until the definitive path removes the persisted file (RemoveLastProvider runs after the
	// lock is released, so observing the file's absence is race-free and confirms full processing).
	deadline := time.Now().Add(10 * time.Second)
	var removed bool
	for time.Now().Before(deadline) {
		if _, err := os.Stat("last_provider.json"); os.IsNotExist(err) {
			removed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !removed {
		t.Fatalf("expected last_provider.json removed after definitive crash")
	}

	s := ctrl.Status()
	if s.Provider != nil {
		t.Fatalf("expected provider cleared after definitive crash, got: %+v", s.Provider)
	}
	if !strings.Contains(s.LastError, "guests cannot create rooms") {
		t.Errorf("expected LastError to embed captured line, got: %q", s.LastError)
	}
}

func TestControllerKeepsProviderOnTransientCrash(t *testing.T) {
	dir := chdirTemp(t)
	script := writeFakeOlcrtc(t, dir, "echo '429 too many requests' >&2\nexit 1\n")

	l := logger.New(true)
	cfg := &config.Config{IsClient: true, SleepOnError: 1, OlcrtcExe: script, LastProviderFile: "last_provider.json"}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	controller.SaveLastProvider("last_provider.json", provider.Provider{Kind: "wbstream", RoomID: "room-1"}, l)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)
	ctrl.SetProvider(&provider.Provider{Kind: "wbstream", RoomID: "room-1"}, true)

	// Poll until at least one transient crash has been processed.
	deadline := time.Now().Add(10 * time.Second)
	var s controller.Status
	for time.Now().Before(deadline) {
		s = ctrl.Status()
		if s.LastExitCode != nil && s.Failures > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if s.LastExitCode == nil {
		t.Fatal("expected at least one crash to be processed")
	}
	if s.Provider == nil {
		t.Fatal("expected provider retained after transient crash")
	}
	if s.CrashFailures == 0 {
		t.Error("expected crashFailures > 0 after a subprocess crash")
	}
	if _, err := os.Stat("last_provider.json"); err != nil {
		t.Fatalf("expected last_provider.json to survive transient crash, got err=%v", err)
	}
}

func TestControllerEnvFailureKeepsProvider(t *testing.T) {
	chdirTemp(t)

	l := logger.New(true)
	// OlcrtcExe points at a non-existent binary: cmd.Start will fail (host-side fault).
	cfg := &config.Config{IsClient: true, SleepOnError: 1, OlcrtcExe: "/nonexistent/olcrtc-binary", LastProviderFile: "last_provider.json"}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	// Pre-create a previously-good provider file; the controller loads it on Start.
	controller.SaveLastProvider("last_provider.json", provider.Provider{Kind: "wbstream", RoomID: "room-1"}, l)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl.Start(ctx)

	// Poll until at least one start fault has incremented failures.
	deadline := time.Now().Add(12 * time.Second)
	var s controller.Status
	for time.Now().Before(deadline) {
		s = ctrl.Status()
		if s.Failures > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if s.Failures == 0 {
		t.Fatal("expected failures to increment on host-side start fault")
	}
	if s.CrashFailures != 0 {
		t.Errorf("expected crashFailures to stay 0 on env start-fault, got %d", s.CrashFailures)
	}
	if _, err := os.Stat("last_provider.json"); err != nil {
		t.Fatalf("expected last_provider.json to survive env start-fault indefinitely, got err=%v", err)
	}
}

func TestClearProviderRemovesFile(t *testing.T) {
	chdirTemp(t)

	l := logger.New(true)
	cfg := &config.Config{IsClient: true, SleepOnError: 1, LastProviderFile: "last_provider.json"}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	controller.SaveLastProvider("last_provider.json", provider.Provider{Kind: "wbstream", RoomID: "room-1"}, l)
	ctrl.SetProvider(&provider.Provider{Kind: "wbstream", RoomID: "room-1"}, false)

	if ctrl.Status().Provider == nil {
		t.Fatal("expected provider set before ClearProvider")
	}

	ctrl.ClearProvider()

	if ctrl.Status().Provider != nil {
		t.Errorf("expected provider nil after ClearProvider, got: %+v", ctrl.Status().Provider)
	}
	if _, err := os.Stat("last_provider.json"); !os.IsNotExist(err) {
		t.Fatalf("expected last_provider.json removed after ClearProvider, got err=%v", err)
	}
}
