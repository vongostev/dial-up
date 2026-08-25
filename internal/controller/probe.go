/*
[2026-08-24] :: 🚀 :: Initial direct-internet HTTP probe
*/

package controller

import (
	"context"
	"io"
	"net/http"
	"time"
)

// probeTimeout bounds a single probe request.
const probeTimeout = 10 * time.Second

// ProbeDirect reports whether direct internet access works (2xx/3xx to url).
func ProbeDirect(ctx context.Context, url string) bool {
	return ProbeDirectWithClient(ctx, url, &http.Client{Timeout: probeTimeout})
}

// ProbeDirectWithClient probes url with an injected client.
func ProbeDirectWithClient(ctx context.Context, url string, client *http.Client) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	// Drain a bounded amount so the connection can be reused; ignore errors.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}
