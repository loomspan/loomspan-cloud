package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("management backend stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	watch, err := engine.InClusterPodWatch()
	if err != nil {
		return err
	}
	transport, err := engine.InClusterTransport()
	if err != nil {
		return err
	}
	console, err := engine.NewConsole(transport, os.Getenv("LOOMSPAN_CONSOLE_API_KEY"))
	if err != nil {
		return err
	}
	observer := engine.NewObserver(console, func(err error) {
		slog.Warn("engine Console connection interrupted", "error", err)
	})
	pods := make(chan *engine.Pod)
	var workers sync.WaitGroup
	workers.Go(func() {
		watch.Run(ctx, pods, func(err error) {
			slog.Warn("engine pod watch interrupted", "error", err)
		})
	})
	workers.Go(func() { observer.Run(ctx, pods) })
	logObservations(ctx, observer)
	workers.Wait()
	return nil
}

func logObservations(ctx context.Context, observer *engine.Observer) {
	for ctx.Err() == nil {
		changed := observer.Changes()
		observation := observer.Current()
		if observation.InstanceID != "" {
			slog.Info("engine observation", "pod", observation.Name, "uid", observation.UID,
				"instance", observation.InstanceID, "connected", observation.Connected,
				"activityConnected", observation.ActivityConnected, "ready", observation.Ready,
				"terminating", observation.Terminating)
		}
		select {
		case <-ctx.Done():
		case <-changed:
		}
	}
}
