/*
[2026-07-12] :: 🚀 :: Initial singbox DelayTest test suite: happy path, bad JSON, non-200, zero delay
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

func TestDelayTestHappy(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		capturedPath = r.URL.Path
		if r.URL.RawQuery == "" {
			t.Error("expected query parameters in delay URL")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"delay":123}`))
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	delay, err := ctrl.DelayTest()
	if err != nil {
		t.Fatalf("DelayTest() = %v", err)
	}
	if delay != 123 {
		t.Errorf("delay = %d, want 123", delay)
	}
	if !strings.Contains(capturedPath, "/proxies/proxy/delay") {
		t.Errorf("path = %q, want to contain /proxies/proxy/delay", capturedPath)
	}
}

func TestDelayTestBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	_, err := ctrl.DelayTest()
	if err == nil {
		t.Fatal("expected error on bad JSON response")
	}
}

func TestDelayTestNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	_, err := ctrl.DelayTest()
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestDelayTestZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]int{"delay": 0})
	}))
	defer srv.Close()

	ctrl := singbox.New(logger.New(true))
	ctrl.ClashAddr = strings.TrimPrefix(srv.URL, "http://")

	_, err := ctrl.DelayTest()
	if err == nil {
		t.Fatal("expected error on zero delay")
	}
}
