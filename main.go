/*
[2026-07-09] :: 🔌 :: Start statusapi loopback endpoint (127.0.0.1:STATUS_PORT) after the controller; bind failure is logged + skipped, never fatal
[2026-07-07] :: 🐛 :: Fixed compile break: pass singbox.New(l) to controller.New as 3rd arg
[2026-07-06] :: 🐛 :: chdir into DATA_DIR (created if missing) so cnc.yaml/srv.yaml/last_provider.json are isolated instead of written to "/" — fixes the root-fs littering when procd starts the bot with CWD=/
[2026-07-02] :: 🚀 :: Initial main entry point
*/

// Package main is the entry point for dial-up.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"dial-up/internal/bot"
	"dial-up/internal/config"
	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
	"dial-up/internal/singbox"
	"dial-up/internal/statusapi"

	"github.com/SevereCloud/vksdk/v3/api"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// instead of the process CWD (which procd leaves at "/", littering the root filesystem).
	// DATA_DIR may be relative (default "data"); resolve it before chdir.
	if cfg.DataDir != "" {
		abs, err := filepath.Abs(cfg.DataDir)
		if err != nil {
			log.Fatalf("Failed to resolve DATA_DIR %q: %v", cfg.DataDir, err)
		}
		if err := os.MkdirAll(abs, 0o750); err != nil {
			log.Fatalf("Failed to create DATA_DIR %s: %v", abs, err)
		}
		if err := os.Chdir(abs); err != nil {
			log.Fatalf("Failed to chdir to DATA_DIR %s: %v", abs, err)
		}
	}

	l := logger.New(cfg.Debug)
	cl := l.With(logger.Function("main"))

	cl.Info("main", "Starting dial-up", logger.Importance(5), logger.Bool("is_client", cfg.IsClient), logger.String("data_dir", cfg.DataDir))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sb := singbox.New(l)
	ctrl := controller.New(cfg, l, sb)
	ctrl.Start(ctx)

	// A bind failure (busy port) is logged and skipped: the panel degrades to the rpcd syslog path.
	statusAddr := "127.0.0.1:" + cfg.StatusPort
	if err := statusapi.New(statusAddr, ctrl.Status, l).Start(ctx); err != nil {
		cl.Warn("main", "Status endpoint not started (panel will use syslog fallback)", logger.Block("StatusEndpoint"), logger.Status("SKIP"), logger.Importance(6), logger.Error(err), logger.String("addr", statusAddr))
	} else {
		cl.Info("main", "Status endpoint started", logger.Block("StatusEndpoint"), logger.Status("OK"), logger.Importance(5), logger.String("addr", statusAddr))
	}

	vk := api.NewVK(cfg.VKToken)
	cl.Info("main", "VK client initialized", logger.Importance(5), logger.Int("token_len", len(cfg.VKToken)))

	b := bot.New(vk, ctrl, cfg, l)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cl.Info("main", "Received OS signal, shutting down", logger.Importance(9))
		cancel()
	}()

	if err := b.Run(ctx); err != nil {
		cl.Error("main", "Bot exited with error", logger.Importance(8), logger.Error(err))
	}

	ctrl.Stop()
	cl.Info("main", "Shutdown complete", logger.Importance(5))
}
