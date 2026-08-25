/*
[2026-08-24] :: 🚀 :: Initial probe tests
*/

package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"dial-up/internal/controller"
)

func TestProbeDirect(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{name: "204", status: http.StatusNoContent, want: true},
		{name: "301", status: http.StatusMovedPermanently, want: true},
		{name: "500", status: http.StatusInternalServerError, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
			}))
			defer srv.Close()

			client := &http.Client{}
			if got := controller.ProbeDirectWithClient(context.Background(), srv.URL, client); got != c.want {
				t.Errorf("ProbeDirect = %v, want %v", got, c.want)
			}
		})
	}

	t.Run("transport error", func(t *testing.T) {
		client := &http.Client{}
		if got := controller.ProbeDirectWithClient(context.Background(), "http://127.0.0.1:1", client); got {
			t.Error("expected false on transport error")
		}
	})
}
