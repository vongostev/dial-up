/*
[2026-07-09] :: 🚀 :: Initial statusapi: loopback GET /status (controller.Status JSON) + /healthz, graceful ctx shutdown, non-fatal bind
*/

package statusapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
)

const logCategory = "statusapi"

// Server serves the controller status snapshot over a loopback HTTP endpoint.
type Server struct {
	addr     string
	statusFn func() controller.Status
	l        logger.Logger
	server   *http.Server
	listener net.Listener
}

// New creates a Server that will serve statusFn snapshots on addr.
func New(addr string, statusFn func() controller.Status, l logger.Logger) *Server {
	return &Server{addr: addr, statusFn: statusFn, l: l}
}

// Start binds the listener and serves requests until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	cl := s.l.With(logger.Function("Server.Start"))

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.addr)
	if err != nil {
		cl.Error(logCategory, "Failed to bind status endpoint", logger.Block("Bind"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err), logger.String("addr", s.addr))
		return fmt.Errorf("%s: %w", ErrStatusBind.Error(), err)
	}
	s.listener = ln
	s.server = &http.Server{
		Addr:              s.addr,
		Handler:           http.HandlerFunc(s.handler),
		ReadHeaderTimeout: 5 * time.Second,
	}
	cl.Info(logCategory, "Status endpoint bound", logger.Block("Bind"), logger.Status("OK"), logger.Importance(5), logger.String("addr", s.addr))

	go func() {
		cl.Debug(logCategory, "Serving status endpoint", logger.Block("Serve"), logger.Status("ATTEMPT"), logger.Importance(4), logger.String("addr", s.addr))
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cl.Error(logCategory, "Status serve exited with error", logger.Block("Serve"), logger.Status("FAIL"), logger.Importance(8), logger.Error(err))
			return
		}
		cl.Info(logCategory, "Status endpoint stopped", logger.Block("Serve"), logger.Status("OK"), logger.Importance(5))
	}()

	go func() {
		<-ctx.Done()
		cl.Info(logCategory, "Context done, shutting down status endpoint", logger.Block("Shutdown"), logger.Status("ATTEMPT"), logger.Importance(7))
		// |:NOTE: ctx is already cancelled here; derive a fresh, non-inherited
		// timeout so Shutdown can drain in-flight requests instead of aborting.
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(sctx); err != nil {
			cl.Error(logCategory, "Status endpoint shutdown error", logger.Block("Shutdown"), logger.Status("FAIL"), logger.Importance(7), logger.Error(err))
			return
		}
		cl.Info(logCategory, "Status endpoint shut down cleanly", logger.Block("Shutdown"), logger.Status("OK"), logger.Importance(5))
	}()

	return nil
}

// Addr returns the bound listen address, or an empty string if not started.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// handler routes status endpoint requests.
func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	cl := s.l.With(logger.Function("Server.handler"))

	switch r.URL.Path {
	case "/status":
		st := s.statusFn()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(st); err != nil {
			cl.Error(logCategory, "Failed to encode status", logger.Block("StatusJSON"), logger.Status("FAIL"), logger.Importance(7), logger.Error(err))
			return
		}
		cl.Debug(logCategory, "Status served", logger.Block("StatusJSON"), logger.Status("OK"), logger.Importance(4))
	case "/healthz":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
