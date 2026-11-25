package engine

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"sync"
	"time"
)

var ErrObservationChanged = errors.New("engine observation changed during refresh")

type Projection struct {
	Pod
	Snapshot
	// Connected starts with the first authenticated snapshot, independently of stream admission.
	Connected         bool
	ActivityConnected bool
	ConnectionID      uint64
}

func (p Projection) PublicationTarget() (Pod, bool) {
	return p.Pod, p.Connected && !p.Pod.Terminating
}

func (p Projection) Confirms(namespace, version string) (NamespaceObservation, bool) {
	row, found := p.Namespaces[namespace]
	return row, p.Connected && p.Ready && !p.Terminating && found &&
		row.PublicationVersion != nil && *row.PublicationVersion == version
}

// Observer associates an authenticated Console snapshot with the single watched Pod.
type Observer struct {
	console *Console
	report  func(error)

	mu      sync.Mutex
	current Projection
	updates chan struct{}
	target  *observationTarget
	session *observationSession
	workers sync.WaitGroup

	retryDelay time.Duration
}

type observationTarget struct {
	pod  Pod
	stop context.CancelFunc
}

type observationSession struct {
	ctx        context.Context
	stop       context.CancelFunc
	connection *consoleConnection
	instance   string
	refresh    chan struct{}
}

func NewObserver(console *Console, report func(error)) *Observer {
	return &Observer{
		console:    console,
		report:     report,
		updates:    make(chan struct{}),
		retryDelay: time.Second,
	}
}

// Changes closes on the next projection update. Take it before reading Current and reacquire it after closure.
// Multiple callers can independently wait for the same update.
func (o *Observer) Changes() <-chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.updates
}

// Current returns a copy of the last observation, even if it is disconnected.
func (o *Observer) Current() Projection {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.copyProjection()
}

// Run consumes current candidate updates from the pod watch until it ends or the context is cancelled.
func (o *Observer) Run(ctx context.Context, pods <-chan *Pod) {
	defer func() {
		o.selectPod(ctx, nil)
		o.workers.Wait()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case pod, open := <-pods:
			if !open {
				return
			}
			o.selectPod(ctx, pod)
		}
	}
}

func (o *Observer) selectPod(ctx context.Context, pod *Pod) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if pod != nil && o.target != nil && o.target.pod.sameTarget(*pod) {
		o.target.pod = *pod
		if o.current.Pod.sameTarget(*pod) && o.current.Pod != *pod {
			o.current.Pod = *pod
			o.changed()
		}
		return
	}
	if o.target != nil {
		o.target.stop()
		o.target = nil
	}
	o.disconnect()
	if pod != nil {
		targetContext, stop := context.WithCancel(ctx)
		target := &observationTarget{pod: *pod, stop: stop}
		o.target = target
		selected := *pod
		o.workers.Go(func() {
			o.observe(targetContext, target, selected)
		})
	}
}

func (o *Observer) observe(ctx context.Context, target *observationTarget, pod Pod) {
	for ctx.Err() == nil {
		err := o.observeConnection(ctx, target, pod)
		if ctx.Err() != nil {
			return
		}
		if err != nil && o.report != nil {
			o.report(err)
		}
		if !retry(ctx, o.retryDelay) {
			return
		}
	}
}

func (o *Observer) observeConnection(ctx context.Context, target *observationTarget, pod Pod) error {
	connection, err := o.console.connect(pod)
	if err != nil {
		return err
	}
	defer connection.client.CloseIdleConnections()
	sessionCtx, stop := context.WithCancel(ctx)
	defer stop()
	snapshot, err := connection.snapshot(sessionCtx)
	if err != nil {
		return err
	}
	session := &observationSession{sessionCtx, stop, connection, snapshot.InstanceID, make(chan struct{}, 1)}

	o.mu.Lock()
	if o.target != target || sessionCtx.Err() != nil {
		o.mu.Unlock()
		return ErrObservationChanged
	}
	o.session = session
	o.current = Projection{
		Pod:          target.pod,
		Snapshot:     snapshot,
		Connected:    true,
		ConnectionID: o.current.ConnectionID + 1,
	}
	o.changed()
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.session == session {
			o.disconnect()
		}
	}()
	return connection.stream(sessionCtx, snapshot.InstanceID, func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.session == session && sessionCtx.Err() == nil {
			o.current.ActivityConnected = true
			o.changed()
		}
	})
}

// Refresh obtains a new snapshot from the active connection. Concurrent refreshes are serialized
// within a connection so an earlier response cannot replace a later observation.
func (o *Observer) Refresh(ctx context.Context) (Projection, error) {
	o.mu.Lock()
	session := o.session
	o.mu.Unlock()
	if session == nil {
		return Projection{}, ErrObservationChanged
	}
	select {
	case session.refresh <- struct{}{}:
		defer func() { <-session.refresh }()
	case <-session.ctx.Done():
		return Projection{}, ErrObservationChanged
	case <-ctx.Done():
		return Projection{}, ctx.Err()
	}
	if session.ctx.Err() != nil {
		return Projection{}, ErrObservationChanged
	}

	requestContext, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(session.ctx, cancel)
	defer stop()
	snapshot, err := session.connection.snapshot(requestContext)

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.session != session || session.ctx.Err() != nil {
		return Projection{}, ErrObservationChanged
	}
	if ctx.Err() != nil {
		return Projection{}, ctx.Err()
	}
	if err != nil || snapshot.InstanceID != session.instance {
		// A possibly restarted or disconnected JVM must be rediscovered with a fresh stream.
		o.disconnect()
		if err != nil {
			return Projection{}, err
		}
		return Projection{}, ErrObservationChanged
	}
	if !reflect.DeepEqual(o.current.Snapshot, snapshot) {
		o.current.Snapshot = snapshot
		o.changed()
	}
	return o.copyProjection(), nil
}

func (o *Observer) disconnect() {
	if o.session == nil {
		return
	}
	o.session.stop()
	o.session = nil
	o.current.Connected = false
	o.current.ActivityConnected = false
	o.changed()
}

func (o *Observer) changed() {
	close(o.updates)
	o.updates = make(chan struct{})
}

func (o *Observer) copyProjection() Projection {
	projection := o.current
	projection.Namespaces = maps.Clone(o.current.Namespaces)
	for namespace, row := range projection.Namespaces {
		if row.PublicationVersion != nil {
			row.PublicationVersion = new(*row.PublicationVersion)
			projection.Namespaces[namespace] = row
		}
	}
	return projection
}
