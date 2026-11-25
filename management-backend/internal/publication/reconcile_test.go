package publication

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

func TestRecoverySerializesWithPublish(t *testing.T) {
	for _, ownerFirst := range []bool{false, true} {
		name := "restore then newer publish"
		if ownerFirst {
			name = "publish then waiting restore"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			service, store, observer, gateway := testService(t, emptyPack(t, "sales"))
			app := store.applications["app"]
			app.state.Current = Publication{"app", "sales", "r1", "old", app.revisions["r1"]}
			app.state.StagedRevision = "r2"
			started, release := make(chan struct{}), make(chan struct{})
			var sent []string
			gateway.onInstall = func(_ context.Context, _ engine.Projection, _ []byte, metadata engineMetadata) installResponse {
				sent = append(sent, metadata.PublicationVersion)
				if len(sent) == 1 {
					close(started)
					<-release
				}
				return installResponse{status: InstallAcknowledged}
			}
			type ownerResult struct {
				result PublishResult
				err    error
			}
			published := make(chan ownerResult, 1)
			owner := func() {
				result, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r2"})
				published <- ownerResult{result, err}
			}
			listed := make(chan struct{})
			store.onList = func() { close(listed) }
			restored := make(chan error, 1)
			reconcile := func() { restored <- service.reconcile(ctx, observer.Current()) }
			if ownerFirst {
				go owner()
				take(t, started)
				go reconcile()
				take(t, listed)
			} else {
				go reconcile()
				take(t, started)
				go owner()
			}
			close(release)
			out, restoreErr := take(t, published), take(t, restored)
			if out.err != nil || restoreErr != nil || out.result.Version == "" || app.state.Current.Version != out.result.Version {
				t.Fatalf("unexpected publication %+v %v and recovery %v", out.result, out.err, restoreErr)
			}
			want := []string{"old", out.result.Version}
			if ownerFirst {
				want = want[1:]
			}
			if !slices.Equal(sent, want) {
				t.Fatalf("sent %v, want %v", sent, want)
			}
		})
	}
}

func TestRecoveryAfterDurableAcknowledgment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pack := workedPack(t)
	service, store, observer, gateway := testService(t, pack)
	app := store.applications["app"]
	app.values = map[string]string{"provider.key": "old-key", "app.region": "east", "records.url": "old-url", "records.lookup-path": "path"}
	gateway.onInstall = func(context.Context, engine.Projection, []byte, engineMetadata) installResponse {
		return installResponse{status: InstallAcknowledged}
	}
	first, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	original := observer.Current()
	markInstalled(&original, "sales", first.Version, "g1", TraceNever)
	observer.set(original)
	app.state.StagedRevision = "r2"
	newer, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Eligible(ctx, "app"); !errors.Is(err, ErrPlacementUnavailable) {
		t.Fatal("the old observation became eligible for the new version")
	}
	app.values["provider.key"] = "new-key"
	app.state.TracePolicy = TraceAlways

	replacement := testProjection(2, false)
	replacement.Pod.UID, replacement.InstanceID = "new-pod", "new-jvm"
	newObserver := newFakeObserver(replacement)
	calls := make(chan engineMetadata, 1)
	gateway.onInstall = func(_ context.Context, target engine.Projection, original []byte, metadata engineMetadata) installResponse {
		if target.Pod.UID != "new-pod" || target.InstanceID != "new-jvm" || !bytes.Equal(original, pack) {
			t.Error("wrong restore target or pack")
		}
		calls <- metadata
		return installResponse{status: InstallAcknowledged}
	}
	restarted := newService(store, service.packs, newObserver, gateway, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		restarted.Run(ctx)
	}()
	meta := take(t, calls)
	take(t, newObserver.refreshed)
	if meta.PublicationVersion != newer.Version || meta.RuntimeSettings.ExecutionTrace.Persistence != TraceAlways ||
		(*meta.VariableValues)["provider.key"] != "new-key" {
		t.Fatalf("restore captured stale saved inputs: %+v", meta)
	}
	if _, err := restarted.Eligible(ctx, "app"); !errors.Is(err, ErrPlacementUnavailable) {
		t.Fatal("restored version was eligible without confirmation")
	}
	markInstalled(&replacement, "sales", newer.Version, "g2", TraceAlways)
	newObserver.set(replacement)
	if _, err := restarted.Eligible(ctx, "app"); !errors.Is(err, ErrPlacementUnavailable) {
		t.Fatal("restored version was eligible on an unready pod")
	}
	replacement.Ready = true
	newObserver.set(replacement)
	placement, err := restarted.Eligible(ctx, "app")
	if err != nil || placement.Version != newer.Version || placement.InstanceID != "new-jvm" ||
		placement.GenerationID != "g2" || placement.TracePersistencePolicy != TraceAlways {
		t.Fatalf("unexpected restored placement: %+v, %v", placement, err)
	}
	cancel()
	take(t, done)
}

func TestReconcile(t *testing.T) {
	ctx := context.Background()
	service, store, observer, gateway := testService(t, emptyPack(t, "sales"))
	add := func(id, namespace, digest string) {
		store.applications[id] = &memoryApplication{state: Selection{
			ApplicationID: id, Namespace: namespace, TracePolicy: TraceOnError,
			Current: Publication{ApplicationID: id, Namespace: namespace, Version: id + "-current", PackDigest: digest},
		}}
	}
	failing, good, matched := "a-failing", "app", "b-matched"
	digest := store.applications[good].revisions["r1"]
	brokenPack := testWritePack(t, service.packs.(*MountedPacks).root.Name(), emptyPack(t, "broken"))
	add(failing, "broken", brokenPack)
	add(good, "sales", digest)
	add(matched, "matching", "unused-digest")
	view := observer.Current()
	markInstalled(&view, "matching", matched+"-current", "g-match", TraceNever)
	markInstalled(&view, "unintended", "foreign", "foreign-gen", TraceAlways)
	observer.set(view)
	var failures []RecoveryFailure
	service.SetRecoveryReporter(func(f RecoveryFailure) { failures = append(failures, f) })
	var sent []string
	gateway.onInstall = func(_ context.Context, _ engine.Projection, _ []byte, meta engineMetadata) installResponse {
		sent = append(sent, meta.Namespace)
		if meta.Namespace == "broken" {
			return installResponse{status: InstallRejected, message: "tenant error"}
		}
		return installResponse{status: InstallAcknowledged}
	}
	if err := service.reconcile(ctx, view); err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || failures[0].ApplicationID != failing || !slices.Equal(sent, []string{"broken", "sales"}) {
		t.Fatalf("failure isolation: %+v, sent %v", failures, sent)
	}
	if p, err := service.Eligible(ctx, matched); err != nil || p.GenerationID != "g-match" {
		t.Fatalf("matching namespace was affected: %+v, %v", p, err)
	}
	if _, err := service.Eligible(ctx, "unintended"); !errors.Is(err, ErrPlacementUnavailable) ||
		observer.Current().Namespaces["unintended"].GenerationID != "foreign-gen" {
		t.Fatal("unintended namespace was mutated or routed")
	}
}

func TestEligibility(t *testing.T) {
	ctx := context.Background()
	service, store, observer, _ := testService(t, emptyPack(t, "sales"))
	app := store.applications["app"]
	app.state.Current = Publication{"app", "sales", "r1", "current", app.revisions["r1"]}
	app.state.TracePolicy = TraceAlways
	view := testProjection(1, true)
	markInstalled(&view, "sales", "current", "g1", TraceOnError)
	observer.set(view)
	p, err := service.Eligible(ctx, "app")
	if err != nil || p.TracePersistencePolicy != TraceOnError {
		t.Fatalf("used saved or default policy instead of namespace policy: %+v %v", p, err)
	}
	for _, breakView := range []func(*engine.Projection){
		func(v *engine.Projection) { v.Ready = false },
		func(v *engine.Projection) { v.Connected = false },
		func(v *engine.Projection) { v.Terminating = true },
		func(v *engine.Projection) { markInstalled(v, "sales", "obsolete", "old", TraceOnError) },
	} {
		broken := view
		broken.Namespaces = observer.Current().Namespaces
		breakView(&broken)
		observer.set(broken)
		if _, err := service.Eligible(ctx, "app"); !errors.Is(err, ErrPlacementUnavailable) {
			t.Fatal("stale projection was eligible")
		}
	}
}

func TestReconcileRetriesOnlyOnFreshAssociation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service, store, observer, gateway := testService(t, emptyPack(t, "sales"))
	app := store.applications["app"]
	app.state.Current = Publication{"app", "sales", "r1", "current", app.revisions["r1"]}
	failures := make(chan RecoveryFailure, 4)
	service.SetRecoveryReporter(func(f RecoveryFailure) { failures <- f })
	var associations []uint64
	gateway.onInstall = func(_ context.Context, target engine.Projection, _ []byte, _ engineMetadata) installResponse {
		associations = append(associations, target.ConnectionID)
		return installResponse{status: InstallRejected, message: "fixture rejection"}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.Run(ctx)
	}()
	take(t, failures)
	same := observer.Current()
	same.ActivityConnected = true
	observer.set(same)
	next := testProjection(2, true)
	next.InstanceID = "reconnected"
	observer.set(next)
	take(t, failures)
	cancel()
	take(t, done)
	if !slices.Equal(associations, []uint64{1, 2}) {
		t.Fatalf("restore attempts targeted %v, expected one per association", associations)
	}
}
