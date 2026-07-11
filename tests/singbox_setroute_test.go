/*
[2026-07-08] :: 🚀 :: Added Status() test suite: alive+proxy, alive+direct, unreachable, bad JSON, non-200
[2026-07-07] :: 🚀 :: Initial singbox SetRoute test suite
*/

package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dial-up/internal/domain/logger"
	"dial-up/internal/singbox"
)

func TestSetRouteValid(t *testing.T) {
	tests := []struct {
		mode string
	}{
		{singbox.ModeProxy},
		{singbox.ModeDirect},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			var capturedPath, capturedBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("expected PUT, got %s", r.Method)
				}
				capturedPath = r.URL.Path

				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				capturedBody = body["name"]

				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			ctrl := singbox.New(logger.New(true))
			ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

			err := ctrl.SetRoute(tt.mode)
			if err != nil {
				t.Fatalf("SetRoute(%q) = %v", tt.mode, err)
			}

			if capturedPath != "/proxies/route-select" {
				t.Errorf("path = %q, want /proxies/route-select", capturedPath)
			}
			if capturedBody != tt.mode {
				t.Errorf("body name = %q, want %q", capturedBody, tt.mode)
			}
		})
	}
}

func TestSetRouteInvalidMode(t *testing.T) {
	ctrl := singbox.New(logger.New(true))

	err := ctrl.SetRoute("invalid")
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "invalid route mode") {
		t.Errorf("error = %q, want to contain 'invalid route mode'", err.Error())
	}
}

func TestSetRouteRetryOnFailure(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	err := ctrl.SetRoute(singbox.ModeProxy)
	if err != nil {
		t.Fatalf("SetRoute failed after retry: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestSetRouteAllRetriesExhausted(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	err := ctrl.SetRoute(singbox.ModeProxy)
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestSetRouteInvalidBody(t *testing.T) {
	ctrl := singbox.New(logger.New(true))

	err := ctrl.SetRoute("nonsense")
	if err == nil {
		t.Fatal("expected error for nonsense mode")
	}
	if !strings.Contains(err.Error(), "invalid route mode") {
		t.Errorf("error = %q, want to contain 'invalid route mode'", err.Error())
	}
}

func TestStatusAliveProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/proxies/route-select" {
			t.Errorf("path = %q, want /proxies/route-select", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"Selector","now":"proxy","all":["proxy","direct"]}`))
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	status, err := ctrl.Status()
	if err != nil {
		t.Fatalf("Status() = %v", err)
	}
	if !status.Alive {
		t.Error("expected Alive=true")
	}
	if status.Route != singbox.ModeProxy {
		t.Errorf("Route = %q, want proxy", status.Route)
	}
}

func TestStatusAliveDirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"Selector","now":"direct","all":["proxy","direct"]}`))
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	status, err := ctrl.Status()
	if err != nil {
		t.Fatalf("Status() = %v", err)
	}
	if !status.Alive {
		t.Error("expected Alive=true")
	}
	if status.Route != singbox.ModeDirect {
		t.Errorf("Route = %q, want direct", status.Route)
	}
}

func TestStatusUnreachable(t *testing.T) {
	ctrl := singbox.New(logger.New(true))
	// Point to a port where nothing listens.
	ctrl.ClashAddr = "127.0.0.1:1"

	status, err := ctrl.Status()
	if err != nil {
		t.Fatalf("Status() = %v", err)
	}
	if status.Alive {
		t.Error("expected Alive=false when API is unreachable")
	}
	if status.Route != "" {
		t.Errorf("Route = %q, want empty", status.Route)
	}
}

func TestStatusBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	status, err := ctrl.Status()
	if err != nil {
		t.Fatalf("Status() = %v", err)
	}
	if !status.Alive {
		t.Error("expected Alive=true (server responded)")
	}
	if status.Route != "" {
		t.Errorf("Route = %q, want empty on bad JSON", status.Route)
	}
}

func TestStatusNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	status, err := ctrl.Status()
	if err != nil {
		t.Fatalf("Status() = %v", err)
	}
	if !status.Alive {
		t.Error("expected Alive=true (server responded, even if 404)")
	}
	if status.Route != "" {
		t.Errorf("Route = %q, want empty on non-200", status.Route)
	}
}
