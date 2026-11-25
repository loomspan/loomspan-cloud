package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
	"github.com/loomspan/loomspan-cloud/management-backend/internal/publication"
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
	startup, cancelStartup := context.WithTimeout(ctx, 10*time.Second)
	store, err := publication.OpenPostgres(startup, os.Getenv("LOOMSPAN_DATABASE_URL"))
	cancelStartup()
	if err != nil {
		return err
	}
	defer store.Close()
	packs, err := publication.OpenMountedPacks(os.Getenv("LOOMSPAN_PACK_ROOT"))
	if err != nil {
		return err
	}
	defer packs.Close()
	client := publication.NewEngineClient(observer, transport)
	publications := publication.NewService(store, packs, observer, client, nil)
	pods := make(chan *engine.Pod)
	var workers sync.WaitGroup
	workers.Go(func() {
		watch.Run(ctx, pods, func(err error) {
			slog.Warn("engine pod watch interrupted", "error", err)
		})
	})
	workers.Go(func() { observer.Run(ctx, pods) })
	workers.Go(func() { publications.Run(ctx) })
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
