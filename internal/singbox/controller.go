/*
[2026-07-12] :: 🚀 :: Added DelayTest to Controller interface + ControllerImpl — GET /proxies/proxy/delay with dedicated 8s HTTP client, returns RTT in ms
[2026-07-08] :: 🚀 :: Added Status struct, Status() to Controller interface and ControllerImpl via GET /proxies/route-select
[2026-07-08] :: 🏗️ :: Removed init.d lifecycle methods (Start/Stop/Status/Restart); sing-box managed exclusively by init.d
[2026-07-07] :: 🚀 :: Added Controller interface + SetRoute via Clash API; clashAddr configurable
[2026-07-02] :: 🚀 :: Initial singbox controller
*/

package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"dial-up/internal/domain/logger"
)

const ModeProxy = "proxy"
const ModeDirect = "direct"

const logCategory = "singbox"

// defaultClashAddr is the default address of the sing-box Clash API external controller.
const defaultClashAddr = "127.0.0.1:9090"

var ErrInvalidRouteMode = errors.New("invalid route mode: must be 'proxy' or 'direct'")

// Status is a snapshot of sing-box state: whether the Clash API is reachable and the active route.
type Status struct {
	Alive bool   `json:"alive"`
	Route string `json:"route"`
}

// Controller defines the sing-box route switching and status query surface consumed by the controller package.
type Controller interface {
	SetRoute(mode string) error
	Status() (Status, error)
	DelayTest() (int, error)
}

// ControllerImpl switches the sing-box selector outbound via Clash API.
type ControllerImpl struct {
	l           logger.Logger
	ClashAddr   string
	httpClient  *http.Client
	delayClient *http.Client
}

// New creates a ControllerImpl with the given logger.
func New(l logger.Logger) *ControllerImpl {
	return &ControllerImpl{
		l:         l,
		ClashAddr: defaultClashAddr,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		delayClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

// SetRoute switches the route-select selector to the given mode.
func (c *ControllerImpl) SetRoute(mode string) error {
	cl := c.l.With(logger.Function("ControllerImpl.SetRoute"))

	if mode != ModeProxy && mode != ModeDirect {
		cl.Error(logCategory, "Invalid route mode", logger.Block("Validate"), logger.Status("FAIL"), logger.Importance(7), logger.String("mode", mode))
		return fmt.Errorf("%w: %s", ErrInvalidRouteMode, mode)
	}

	body, _ := json.Marshal(map[string]string{"name": mode})
	url := fmt.Sprintf("http://%s/proxies/route-select", c.ClashAddr)

	var lastErr error
	for attempt := range 3 {
		cl.Debug(logCategory, "Setting route", logger.Block("ClashPut"), logger.Status("ATTEMPT"), logger.Importance(4), logger.String("mode", mode), logger.Int("attempt", attempt+1))

		req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		if err != nil {
			lastErr = fmt.Errorf("create request: %w", err)
			time.Sleep(time.Second)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http put: %w", err)
			cl.Warn(logCategory, "Clash API request failed, retrying", logger.Block("ClashPut"), logger.Status("FAIL"), logger.Importance(5), logger.Error(err), logger.Int("attempt", attempt+1))
			time.Sleep(time.Second)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusNoContent {
			cl.Info(logCategory, "Route switched", logger.Block("ClashPut"), logger.Status("OK"), logger.Importance(6), logger.String("mode", mode))
			return nil
		}

		lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		cl.Warn(logCategory, "Clash API unexpected status", logger.Block("ClashPut"), logger.Status("FAIL"), logger.Importance(5), logger.Int("status", resp.StatusCode), logger.Int("attempt", attempt+1))
		time.Sleep(time.Second)
	}

	cl.Error(logCategory, "All SetRoute retries exhausted", logger.Block("ClashPut"), logger.Status("FAIL"), logger.Importance(8), logger.Error(lastErr))
	return fmt.Errorf("set route after 3 retries: %w", lastErr)
}

// Status queries the Clash API for the current selector state.
func (c *ControllerImpl) Status() (Status, error) {
	cl := c.l.With(logger.Function("ControllerImpl.Status"))

	url := fmt.Sprintf("http://%s/proxies/route-select", c.ClashAddr)

	cl.Debug(logCategory, "Querying selector status", logger.Block("ClashGet"), logger.Status("ATTEMPT"), logger.Importance(4))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cl.Warn(logCategory, "Failed to create status request", logger.Block("ClashGet"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
		return Status{Alive: false, Route: ""}, nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		cl.Warn(logCategory, "Clash API unreachable", logger.Block("ClashGet"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
		return Status{Alive: false, Route: ""}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cl.Warn(logCategory, "Clash API unexpected status", logger.Block("ClashGet"), logger.Status("FAIL"), logger.Importance(5), logger.Int("status", resp.StatusCode))
		return Status{Alive: true, Route: ""}, nil
	}

	var body struct {
		Now string `json:"now"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		cl.Warn(logCategory, "Failed to parse selector response", logger.Block("ClashGet"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
		return Status{Alive: true, Route: ""}, nil
	}

	cl.Info(logCategory, "Selector status retrieved", logger.Block("ClashGet"), logger.Status("OK"), logger.Importance(5), logger.String("route", body.Now))

	return Status{Alive: true, Route: body.Now}, nil
}

// delayTestURL is the Clash API path for the proxy delay test.
const delayTestURL = "/proxies/proxy/delay?url=http%3A%2F%2Fcp.cloudflare.com&timeout=5000"

// DelayTest measures tunnel RTT through the proxy outbound via the Clash API delay endpoint.
func (c *ControllerImpl) DelayTest() (int, error) {
	cl := c.l.With(logger.Function("ControllerImpl.DelayTest"))

	url := fmt.Sprintf("http://%s%s", c.ClashAddr, delayTestURL)

	cl.Debug(logCategory, "Requesting delay test", logger.Block("DelayGet"), logger.Status("ATTEMPT"), logger.Importance(4))

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cl.Warn(logCategory, "Failed to create delay request", logger.Block("DelayGet"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
		return 0, fmt.Errorf("delay test request: %w", err)
	}

	resp, err := c.delayClient.Do(req)
	if err != nil {
		cl.Warn(logCategory, "Delay test HTTP failed", logger.Block("DelayGet"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
		return 0, fmt.Errorf("delay test: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cl.Warn(logCategory, "Delay test unexpected status", logger.Block("DelayGet"), logger.Status("FAIL"), logger.Importance(5), logger.Int("status", resp.StatusCode))
		return 0, fmt.Errorf("delay test: unexpected status %d", resp.StatusCode)
	}

	var body struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<12)).Decode(&body); err != nil {
		cl.Warn(logCategory, "Failed to parse delay response", logger.Block("DelayGet"), logger.Status("FAIL"), logger.Importance(6), logger.Error(err))
		return 0, fmt.Errorf("delay test parse: %w", err)
	}

	if body.Delay <= 0 {
		cl.Warn(logCategory, "Delay test returned invalid delay", logger.Block("DelayGet"), logger.Status("FAIL"), logger.Importance(5), logger.Int("delay", body.Delay))
		return 0, fmt.Errorf("delay test: invalid delay %d", body.Delay)
	}

	cl.Info(logCategory, "Delay test completed", logger.Block("DelayGet"), logger.Status("OK"), logger.Importance(5), logger.Int("delay", body.Delay))

	return body.Delay, nil
}
