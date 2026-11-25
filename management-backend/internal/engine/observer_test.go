package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestObserverRefreshProjectsNamespacePolicy(t *testing.T) {
	var published atomic.Bool
	streamAllowed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, consolePath) {
		case "/instance":
			rows := []NamespaceObservation{{"default", "default-1", nil, "ONERROR"}}
			if published.Load() {
				rows = append(rows, NamespaceObservation{"app", "app-1", new("reviewed"), "NEVER"})
			}
			writeSnapshot(w, instanceOne, "ONERROR", rows)
		case "/active-executions":
			writeBaseline(w, instanceOne)
		case "/activity":
			if waitContext(r.Context(), streamAllowed) {
				writeHandshake(w, instanceOne)
				<-r.Context().Done()
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	observer, pods := startTestObserver(ctx, t, server)
	pod := testObservedPod()
	pod.Ready = false
	pods <- &pod
	early := awaitProjection(t, ctx, observer, func(p Projection) bool { return p.Connected })
	if candidate, ok := early.PublicationTarget(); !ok || candidate.Ready || early.ActivityConnected {
		t.Fatalf("first publication depended on readiness or activity admission: %+v", early)
	}
	if _, ok := early.Confirms("app", "reviewed"); ok {
		t.Fatal("an absent application was eligible for routing")
	}
	changedOne, changedTwo := observer.Changes(), observer.Changes()
	published.Store(true)
	fresh, err := observer.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []<-chan struct{}{changedOne, changedTwo} {
		if !waitContext(ctx, changed) {
			t.Fatal("projection change was not broadcast")
		}
	}
	row := fresh.Namespaces["app"]
	if row.GenerationID != "app-1" || *row.PublicationVersion != "reviewed" ||
		row.TracePersistencePolicy != "NEVER" || fresh.TracePersistencePolicy != "ONERROR" {
		t.Fatalf("refresh mixed namespace and default policy: row=%+v projection=%+v", row, fresh)
	}
	if _, ok := fresh.Confirms("app", "reviewed"); ok {
		t.Fatal("an unready pod was eligible for routing")
	}
	*row.PublicationVersion = "changed by caller"
	delete(fresh.Namespaces, "app")
	close(streamAllowed)
	ready := pod
	ready.Ready = true
	pods <- &ready
	confirmed := awaitProjection(t, ctx, observer, func(p Projection) bool {
		_, ok := p.Confirms("app", "reviewed")
		return p.ActivityConnected && ok
	})
	if _, ok := confirmed.Confirms("app", "older"); ok {
		t.Fatal("a different publication version was eligible")
	}
	terminating := ready
	terminating.Terminating = true
	pods <- &terminating
	last := awaitProjection(t, ctx, observer, func(p Projection) bool { return p.Terminating })
	if _, ok := last.PublicationTarget(); ok {
		t.Fatal("terminating pod was eligible to publish")
	}
	if _, ok := last.Confirms("app", "reviewed"); ok {
		t.Fatal("terminating pod was eligible to invoke")
	}
}

func TestObserverRetainsDisconnectedObservation(t *testing.T) {
	var snapshots, streams atomic.Int32
	doDisconnect, nextSnapshot := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, consolePath) {
		case "/instance":
			if snapshots.Add(1) > 1 && !waitContext(r.Context(), nextSnapshot) {
				return
			}
			writeSnapshot(w, instanceOne, "ONERROR", testInstalledNamespace())
		case "/active-executions":
			writeBaseline(w, instanceOne)
		case "/activity":
			writeHandshake(w, instanceOne)
			if streams.Add(1) == 1 {
				waitContext(r.Context(), doDisconnect)
			} else {
				<-r.Context().Done()
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	observer, pods := startTestObserver(ctx, t, server)
	pod := testObservedPod()
	pods <- &pod
	first := awaitProjection(t, ctx, observer, func(p Projection) bool { return p.ActivityConnected })
	close(doDisconnect)
	last := awaitProjection(t, ctx, observer, func(p Projection) bool { return !p.Connected })
	if last.InstanceID != first.InstanceID || last.Pod != first.Pod ||
		last.Namespaces["app"].GenerationID != first.Namespaces["app"].GenerationID {
		t.Fatalf("last snapshot was lost on disconnect: %+v", last)
	}
	if _, ok := last.Confirms("app", "reviewed"); ok {
		t.Fatal("old observation qualifies while disconnected")
	}
	close(nextSnapshot)
	reconnected := awaitProjection(t, ctx, observer, func(p Projection) bool { return p.ActivityConnected })
	if reconnected.InstanceID != first.InstanceID || reconnected.ConnectionID == first.ConnectionID {
		t.Fatalf("fresh connection epoch not associated: %+v", reconnected)
	}
	if _, ok := reconnected.Confirms("app", "reviewed"); !ok {
		t.Fatal("fresh matching snapshot did not enable routing")
	}
}

func TestObserverRejectsSupersededWork(t *testing.T) {
	var runningInstance atomic.Value
	runningInstance.Store(instanceOne)
	var snapshotCount atomic.Int32
	delayedRefresh, finishRefresh := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		instance := runningInstance.Load().(string)
		switch strings.TrimPrefix(r.URL.Path, consolePath) {
		case "/instance":
			request := snapshotCount.Add(1)
			if request == 4 {
				close(delayedRefresh)
				// Deliberately return the old JVM response even after request cancellation.
				<-finishRefresh
			}
			rows := testInstalledNamespace()
			if instance == instanceTwo {
				rows = []NamespaceObservation{}
			}
			writeSnapshot(w, instance, "ONERROR", rows)
		case "/active-executions":
			writeBaseline(w, instance)
		case "/activity":
			writeHandshake(w, instance)
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	defer func() {
		select {
		case <-finishRefresh:
		default:
			close(finishRefresh)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	observer, pods := startTestObserver(ctx, t, server)
	pod := testObservedPod()
	pods <- &pod
	first := awaitProjection(t, ctx, observer, func(p Projection) bool { return p.ActivityConnected })

	runningInstance.Store(instanceTwo)
	if _, err := observer.Refresh(ctx); !errors.Is(err, ErrObservationChanged) {
		t.Fatalf("refresh failed to invalidate old JVM: %v", err)
	}
	restarted := awaitProjection(t, ctx, observer, func(p Projection) bool {
		return p.ActivityConnected && p.InstanceID == instanceTwo
	})
	if restarted.UID != first.UID || restarted.ConnectionID <= first.ConnectionID {
		t.Fatalf("JVM restart did not preserve pod identity: %+v", restarted)
	}
	if _, ok := restarted.Confirms("app", "reviewed"); ok {
		t.Fatal("old publication was associated with restarted JVM")
	}

	refreshResult := make(chan error, 1)
	go func() {
		_, err := observer.Refresh(ctx)
		refreshResult <- err
	}()
	if !waitContext(ctx, delayedRefresh) {
		t.Fatal("delayed refresh never started")
	}
	runningInstance.Store(instanceOne)
	replacement := pod
	replacement.UID = "new-uid-reusing-name-and-address"
	pods <- &replacement
	replaced := awaitProjection(t, ctx, observer, func(p Projection) bool {
		return p.ActivityConnected && p.UID == replacement.UID
	})
	close(finishRefresh)
	select {
	case err := <-refreshResult:
		if !errors.Is(err, ErrObservationChanged) {
			t.Fatalf("a superseded refresh reported a result: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("superseded refresh did not finish")
	}
	latest := observer.Current()
	if latest.UID != replacement.UID || latest.InstanceID != instanceOne ||
		latest.ConnectionID != replaced.ConnectionID {
		t.Fatalf("old connection overwrote replacement: %+v", latest)
	}
}

func TestObserverSerializesConcurrentRefreshes(t *testing.T) {
	var snapshotCount atomic.Int32
	firstRefresh, allowFirst := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, consolePath) {
		case "/instance":
			number := snapshotCount.Add(1)
			if number == 2 {
				close(firstRefresh)
				if !waitContext(r.Context(), allowFirst) {
					return
				}
			}
			version := "initial"
			if number == 2 {
				version = "second"
			} else if number == 3 {
				version = "third"
			}
			writeSnapshot(w, instanceOne, "ONERROR",
				[]NamespaceObservation{{"app", "app-1", &version, "NEVER"}})
		case "/active-executions":
			writeBaseline(w, instanceOne)
		case "/activity":
			writeHandshake(w, instanceOne)
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	observer, pods := startTestObserver(ctx, t, server)
	pod := testObservedPod()
	pods <- &pod
	awaitProjection(t, ctx, observer, func(p Projection) bool { return p.ActivityConnected })
	refresh := func() <-chan Projection {
		done := make(chan Projection, 1)
		go func() {
			result, err := observer.Refresh(ctx)
			if err != nil {
				t.Error(err)
			}
			done <- result
		}()
		return done
	}
	first := refresh()
	if !waitContext(ctx, firstRefresh) {
		t.Fatal("first refresh never started")
	}
	second := refresh()
	close(allowFirst)
	for i, expected := range []string{"second", "third"} {
		result := []<-chan Projection{first, second}[i]
		select {
		case projection := <-result:
			if _, ok := projection.Confirms("app", expected); !ok {
				t.Fatalf("concurrent refresh returned unexpected version: %+v", projection)
			}
		case <-ctx.Done():
			t.Fatal("refresh timed out")
		}
	}
}

func testObservedPod() Pod {
	return Pod{Namespace: EngineNamespace, Name: "engine", UID: "old-uid", Address: "10.42.0.1", Ready: true}
}

func testInstalledNamespace() []NamespaceObservation {
	return []NamespaceObservation{{"app", "app-1", new("reviewed"), "NEVER"}}
}

func startTestObserver(ctx context.Context, t *testing.T, server *httptest.Server) (*Observer, chan<- *Pod) {
	t.Helper()
	observer := NewObserver(testConsole(t, server), nil)
	observer.retryDelay = time.Millisecond
	pods := make(chan *Pod)
	go observer.Run(ctx, pods)
	return observer, pods
}

func awaitProjection(t *testing.T, ctx context.Context, observer *Observer, condition func(Projection) bool) Projection {
	t.Helper()
	for {
		changes := observer.Changes()
		projection := observer.Current()
		if condition(projection) {
			return projection
		}
		if !waitContext(ctx, changes) {
			t.Fatalf("observation timed out: %+v", projection)
		}
	}
}
