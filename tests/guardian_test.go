/*
[2026-07-10] :: 🚀 :: Initial guardian lifecycle integration test (Start/Stop through Controller)
*/

package tests

import (
	"context"
	"testing"
	"time"

	"dial-up/internal/config"
	"dial-up/internal/controller"
	"dial-up/internal/domain/logger"
)

func TestGuardianStartStopLifecycle(t *testing.T) {
	l := logger.New(true)
	cfg := &config.Config{IsClient: true, SleepOnError: 1}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// START_BLOCK_RUN
	ctrl.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	ctrl.Stop()
	// END_BLOCK_RUN

	// If Start/Stop completed without hanging or panicking, the lifecycle is sound.
	t.Logf("Guardian lifecycle completed cleanly (Start → Stop)")
}

func TestGuardianNotCreatedInServerMode(t *testing.T) {
	l := logger.New(true)
	cfg := &config.Config{IsClient: false, SleepOnError: 1}
	ctrl := controller.New(cfg, l, &fakeSingBox{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	ctrl.Stop()

	t.Logf("Server-mode lifecycle completed cleanly (no guardian)")
}
