/*
[2026-07-12] :: 🚀 :: Added /route handler tests: valid (direct+proxy), invalid mode, routeFn error, nil routeFn, wrong method
[2026-07-09] :: 🚀 :: Initial statusapi test suite
*/

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
	"dial-up/internal/provider"
	"dial-up/internal/singbox"
	"dial-up/internal/statusapi"
)

// newStatusServer builds a Server on an ephemeral loopback port with the given snapshot and optional routeFn.
func newStatusServer(t *testing.T, st controller.Status, routeFn func(string) error) (*statusapi.Server, context.CancelFunc) {
	t.Helper()
	l := logger.New(true)
	ctx, cancel := context.WithCancel(context.Background())
	s := statusapi.New("127.0.0.1:0", func() controller.Status { return st }, routeFn, l)
	if err := s.Start(ctx); err != nil {
		cancel()
		t.Fatalf("Start failed: %v", err)
	}
	return s, cancel
}

// doGet performs a GET, fully reads+closes the body, and returns
// (statusCode, contentType, bodyText) so the response never leaks.
func doGet(t *testing.T, url string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request %s failed: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(body)
}

func TestStatusHandlerNilProvider(t *testing.T) {
	want := controller.Status{
		TunnelAlive:   true,
		Failures:      2,
		CrashFailures: 1,
		LastError:     "boom",
		Provider:      nil,
		PingDNS:       "14ms",
	}
	s, cancel := newStatusServer(t, want, nil)
	defer cancel()

	code, ct, body := doGet(t, "http://"+s.Addr()+"/status")
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", code, body)
	}
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q, want json", ct)
	}

	var got controller.Status
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode failed: %v; body=%s", err, body)
	}
	if got.Provider != nil {
		t.Errorf("provider = %v, want nil", got.Provider)
	}
	if got.TunnelAlive != want.TunnelAlive || got.Failures != want.Failures || got.CrashFailures != want.CrashFailures || got.LastError != want.LastError {
		t.Errorf("decoded status mismatch: got %+v, want %+v", got, want)
	}
}

func TestStatusHandlerSetProvider(t *testing.T) {
	want := controller.Status{
		TunnelAlive: false,
		Provider:    &provider.Provider{Kind: provider.ProviderWbStream, RoomID: "019f33d5-c73d-7a09-ba85-b874bd1fceab"},
	}
	s, cancel := newStatusServer(t, want, nil)
	defer cancel()

	code, _, body := doGet(t, "http://"+s.Addr()+"/status")
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", code, body)
	}
	if !strings.Contains(body, `"kind":"wbstream"`) || !strings.Contains(body, `"room_id":"019f33d5-c73d-7a09-ba85-b874bd1fceab"`) {
		t.Fatalf("body does not contain tagged provider JSON: %s", body)
	}

	var got controller.Status
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode failed: %v; body=%s", err, body)
	}
	if got.Provider == nil || got.Provider.Kind != provider.ProviderWbStream || got.Provider.RoomID != "019f33d5-c73d-7a09-ba85-b874bd1fceab" {
		t.Errorf("decoded provider = %+v, want wbstream/019f33d5...", got.Provider)
	}
}

func TestHealthZ(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{}, nil)
	defer cancel()

	code, _, body := doGet(t, "http://"+s.Addr()+"/healthz")
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	if !strings.Contains(body, "ok") {
		t.Fatalf("body = %q, want to contain 'ok'", body)
	}
}

func TestUnknownPath(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{}, nil)
	defer cancel()

	code, _, _ := doGet(t, "http://"+s.Addr()+"/does-not-exist")
	if code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", code)
	}
}

func TestRouteHandlerValid(t *testing.T) {
	var invoked string
	routeFn := func(mode string) error {
		invoked = mode
		return nil
	}
	s, cancel := newStatusServer(t, controller.Status{}, routeFn)
	defer cancel()

	tests := []struct {
		mode string
	}{
		{mode: "direct"},
		{mode: "proxy"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			invoked = ""
			body := `{"mode":"` + tt.mode + `"}`
			req, _ := http.NewRequest(http.MethodPut, "http://"+s.Addr()+"/route", bytes.NewReader([]byte(body)))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status code = %d, want 200", resp.StatusCode)
			}
			b, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(b), `"ok":true`) {
				t.Fatalf("response body missing ok: %s", string(b))
			}
			if invoked != tt.mode {
				t.Errorf("routeFn invoked with %q, want %q", invoked, tt.mode)
			}
		})
	}
}

func TestRouteHandlerInvalidMode(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{}, func(mode string) error { return nil })
	defer cancel()

	body := `{"mode":"bogus"}`
	req, _ := http.NewRequest(http.MethodPut, "http://"+s.Addr()+"/route", bytes.NewReader([]byte(body)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "invalid mode") {
		t.Fatalf("response body missing invalid mode error: %s", string(b))
	}
}

func TestRouteHandlerRouteFnError(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{}, func(mode string) error { return errors.New("boom") })
	defer cancel()

	body := `{"mode":"direct"}`
	req, _ := http.NewRequest(http.MethodPut, "http://"+s.Addr()+"/route", bytes.NewReader([]byte(body)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want 500", resp.StatusCode)
	}
}

func TestRouteHandlerNilFn(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{}, nil)
	defer cancel()

	body := `{"mode":"direct"}`
	req, _ := http.NewRequest(http.MethodPut, "http://"+s.Addr()+"/route", bytes.NewReader([]byte(body)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", resp.StatusCode)
	}
}

func TestRouteHandlerBadMethod(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{}, func(mode string) error { return nil })
	defer cancel()

	code, _, body := doGet(t, "http://"+s.Addr()+"/route")
	if code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want 405; body=%s", code, body)
	}
}

func TestStatusServeOverListener(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{SingBoxRoute: singbox.ModeProxy}, nil)
	defer cancel()

	if s.Addr() == "" {
		t.Fatal("Addr() empty after Start")
	}
	code, _, body := doGet(t, "http://"+s.Addr()+"/status")
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", code, body)
	}
	if !strings.Contains(body, `"sing_box_route":"proxy"`) {
		t.Fatalf("body does not contain sing_box_route: %s", body)
	}
}

func TestStartBadPort(t *testing.T) {
	l := logger.New(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := statusapi.New("127.0.0.1:bogus", func() controller.Status { return controller.Status{} }, nil, l)
	if err := s.Start(ctx); err == nil {
		t.Fatal("Start with bogus address should return an error")
	}
	if s.Addr() != "" {
		t.Errorf("Addr() = %q after failed bind, want empty", s.Addr())
	}
}

func TestGracefulShutdown(t *testing.T) {
	s, cancel := newStatusServer(t, controller.Status{}, nil)
	addr := s.Addr()

	// Sanity: server is up
	if _, _, hb := doGet(t, "http://"+addr+"/healthz"); hb == "" || !strings.Contains(hb, "ok") {
		t.Fatalf("server not reachable before shutdown: %q", hb)
	}

	cancel()

	// Give the shutdown goroutine time to close the listener.
	deadline := time.Now().Add(3 * time.Second)
	var refused bool
	for time.Now().Before(deadline) {
		ctx, cancelReq := context.WithTimeout(context.Background(), 500*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
		cancelReq()
		if err != nil {
			refused = true
			break
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	if !refused {
		t.Fatal("expected connection to be refused after shutdown")
	}
}
