package publication

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

var testOwner = Actor{Issuer: "cloud", Subject: "owner"}

func authorizeOwner(_ context.Context, actor Actor, _ string, _ Permission) error {
	if actor != testOwner {
		return ErrAccessDenied
	}
	return nil
}

type memoryApplication struct {
	state     Selection
	revisions map[string]string
	values    map[string]string
}

type memoryStore struct {
	mu           sync.Mutex
	applications map[string]*memoryApplication
	attempts     []InstallAttempt
	finished     []InstallStatus
	lookups      [][]string
	onList       func()
}

func (s *memoryStore) OpenSelection(_ context.Context, query SelectionQuery, lookup func(Selection) ([]string, error)) (Selection, error) {
	s.mu.Lock()
	app := s.applications[query.ApplicationID]
	if app == nil {
		s.mu.Unlock()
		return Selection{}, ErrSelectionNotFound
	}
	selected := app.state
	selected.RevisionID = query.RevisionID
	selected.PackDigest = app.revisions[query.RevisionID]
	if query.RevisionID == "" {
		selected.RevisionID, selected.PackDigest = app.state.Current.RevisionID, app.state.Current.PackDigest
	}
	values := maps.Clone(app.values)
	s.mu.Unlock()
	if selected.PackDigest == "" {
		return Selection{}, ErrSelectionNotFound
	}
	names, err := lookup(selected)
	if err != nil {
		return Selection{}, err
	}
	s.mu.Lock()
	s.lookups = append(s.lookups, slices.Clone(names))
	s.mu.Unlock()
	selected.SavedValues = make(map[string]string, len(names))
	for _, name := range names {
		if value, present := values[name]; present {
			selected.SavedValues[name] = value
		}
	}
	return selected, nil
}

func (s *memoryStore) CurrentPublication(_ context.Context, id string) (Publication, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	app := s.applications[id]
	if app == nil || app.state.Current.Version == "" {
		return Publication{}, ErrSelectionNotFound
	}
	return app.state.Current, nil
}

func (s *memoryStore) ListCurrentPublications(_ context.Context) ([]Publication, error) {
	s.mu.Lock()
	var result []Publication
	for _, app := range s.applications {
		if app.state.Current.Version != "" {
			result = append(result, app.state.Current)
		}
	}
	onList := s.onList
	s.mu.Unlock()
	if onList != nil {
		onList()
	}
	slices.SortFunc(result, func(a, b Publication) int { return strings.Compare(a.ApplicationID, b.ApplicationID) })
	return result, nil
}

func (s *memoryStore) StartInstall(_ context.Context, attempt InstallAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = append(s.attempts, attempt)
	return nil
}

func (s *memoryStore) CompleteInstall(_ context.Context, attempt InstallAttempt, status InstallStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished = append(s.finished, status)
	if attempt.Purpose == InstallPublish && status == InstallAcknowledged {
		s.applications[attempt.Publication.ApplicationID].state.Current = attempt.Publication
	}
	return nil
}

type fakeObserver struct {
	mu        sync.Mutex
	view      engine.Projection
	change    chan struct{}
	refreshed chan struct{}
}

func newFakeObserver(initial engine.Projection) *fakeObserver {
	initial.Namespaces = maps.Clone(initial.Namespaces)
	return &fakeObserver{view: initial, change: make(chan struct{}), refreshed: make(chan struct{}, 32)}
}

func (o *fakeObserver) Current() engine.Projection {
	o.mu.Lock()
	defer o.mu.Unlock()
	view := o.view
	view.Namespaces = maps.Clone(view.Namespaces)
	return view
}

func (o *fakeObserver) Changes() <-chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.change
}

func (o *fakeObserver) Refresh(ctx context.Context) (engine.Projection, error) {
	select {
	case o.refreshed <- struct{}{}:
	case <-ctx.Done():
		return engine.Projection{}, ctx.Err()
	}
	return o.Current(), nil
}

func (o *fakeObserver) set(next engine.Projection) {
	o.mu.Lock()
	defer o.mu.Unlock()
	next.Namespaces = maps.Clone(next.Namespaces)
	o.view = next
	close(o.change)
	o.change = make(chan struct{})
}

func testProjection(connection uint64, ready bool) engine.Projection {
	return engine.Projection{
		Pod:       engine.Pod{Namespace: "engines", Name: "engine", UID: "pod", Address: "127.0.0.1", Ready: ready},
		Snapshot:  engine.Snapshot{InstanceID: "instance", Namespaces: make(map[string]engine.NamespaceObservation), TracePersistencePolicy: TraceNever},
		Connected: true, ConnectionID: connection,
	}
}

func markInstalled(observation *engine.Projection, namespace, version, generation, policy string) {
	observation.Namespaces[namespace] = engine.NamespaceObservation{
		Namespace: namespace, PublicationVersion: &version, GenerationID: generation, TracePersistencePolicy: policy,
	}
}

type fakeGateway struct {
	onValidate func(context.Context, engine.Projection, []byte, engineMetadata) (engineReport, *ValidationUnavailable)
	onInstall  func(context.Context, engine.Projection, []byte, engineMetadata) installResponse
}

func (g *fakeGateway) validate(ctx context.Context, target engine.Projection, pack []byte, meta engineMetadata) (engineReport, *ValidationUnavailable) {
	return g.onValidate(ctx, target, pack, meta)
}

func (g *fakeGateway) install(ctx context.Context, target engine.Projection, pack []byte, meta engineMetadata) installResponse {
	return g.onInstall(ctx, target, pack, meta)
}

func testService(t *testing.T, authored []byte) (*Service, *memoryStore, *fakeObserver, *fakeGateway) {
	t.Helper()
	dir := t.TempDir()
	packs, err := OpenMountedPacks(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { packs.Close() })
	digest := testWritePack(t, dir, authored)
	store := &memoryStore{applications: map[string]*memoryApplication{
		"app": {state: Selection{ApplicationID: "app", Namespace: "sales", StagedRevision: "r1", TracePolicy: TraceNever},
			revisions: map[string]string{"r1": digest, "r2": digest}, values: make(map[string]string)},
	}}
	observer := newFakeObserver(testProjection(1, true))
	gateway := &fakeGateway{}
	return newService(store, packs, observer, gateway, AuthorizerFunc(authorizeOwner)), store, observer, gateway
}

func workedPack(t *testing.T) []byte {
	t.Helper()
	base := "testdata/framework-variables/worked"
	return testPack(t, zip.Deflate,
		testZipMember{"sales/loomspan/execution.yaml", readFixture(t, filepath.Join(base, "loomspan/execution.yaml"))},
		testZipMember{"sales/loomspan/skills/read-record.yaml", readFixture(t, filepath.Join(base, "loomspan/skills/read-record.yaml"))},
	)
}

func emptyPack(t *testing.T, namespace string) []byte {
	t.Helper()
	return testPack(t, zip.Store, testZipMember{namespace + "/loomspan/execution.yaml", []byte("loomspan: {}")})
}

func take[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for test checkpoint")
		var zero T
		return zero
	}
}

func TestValidate(t *testing.T) {
	t.Run("completed validation retains the requested inputs", func(t *testing.T) {
		data := workedPack(t)
		service, store, _, gateway := testService(t, data)
		app := store.applications["app"]
		app.values["app.region"] = "\u00a0"
		dispatch := make(chan engineMetadata, 1)
		release := make(chan struct{})
		gateway.onValidate = func(_ context.Context, _ engine.Projection, pack []byte, meta engineMetadata) (engineReport, *ValidationUnavailable) {
			if !bytes.Equal(pack, data) {
				t.Error("validation rewrote the pack")
			}
			dispatch <- meta
			<-release
			return engineReport{Valid: true, Issues: []EngineIssue{{Severity: "WARNING", SourceName: executionSource}},
				Skills: []ValidatedSkill{{Namespace: "sales", LocalName: "read-record", Kind: "EXECUTOR"}}}, nil
		}
		type outcome struct {
			result ValidationResult
			err    error
		}
		finished := make(chan outcome, 1)
		go func() {
			result, err := service.Validate(context.Background(), testOwner, "app", "r1")
			finished <- outcome{result, err}
		}()
		meta := take(t, dispatch)
		store.mu.Lock()
		app.state.StagedRevision, app.state.TracePolicy = "r2", TraceAlways
		store.mu.Unlock()
		close(release)
		out := take(t, finished)
		if out.err != nil || !out.result.Valid || len(out.result.PublicationBlockers) != 6 ||
			len(out.result.Issues) != 1 || len(out.result.Skills) != 1 || out.result.RevisionID != "r1" ||
			out.result.RuntimeSettings.ExecutionTrace.Persistence != TraceNever || meta.RuntimeSettings != out.result.RuntimeSettings ||
			meta.VariableValues != nil || meta.Namespace != "sales" {
			t.Fatalf("unexpected result: %+v, %v", out.result, out.err)
		}
		if out.result.PublicationBlockers[1].Reason != "blank-value" ||
			out.result.PublicationBlockers[5] != (VariableIssue{"missing-value", "loomspan/skills/read-record.yaml", "config.path"}) {
			t.Fatalf("unexpected blockers: %+v", out.result.PublicationBlockers)
		}
		if !slices.Equal(store.lookups[0], []string{"provider.key", "app.region", "records.url", "records.lookup-path"}) {
			t.Fatalf("unexpected selected names: %v", store.lookups[0])
		}
		store.mu.Lock()
		app.state.StagedRevision = "r1"
		app.values = map[string]string{
			"provider.key": "key", "app.region": "west", "records.url": " https://example.test ",
			"records.lookup-path": "${untouched}", "extra": "must not be sent",
		}
		store.mu.Unlock()
		gateway.onInstall = func(_ context.Context, _ engine.Projection, original []byte, meta engineMetadata) installResponse {
			if meta.RuntimeSettings.ExecutionTrace.Persistence != TraceAlways || !bytes.Equal(original, data) ||
				meta.VariableValues == nil || len(*meta.VariableValues) != 4 ||
				(*meta.VariableValues)["records.lookup-path"] != "${untouched}" ||
				(*meta.VariableValues)["records.url"] != " https://example.test " {
				t.Error("publish did not use the exact saved inputs")
			}
			return installResponse{status: InstallAcknowledged}
		}
		if _, err := service.Publish(context.Background(), testOwner, "app", PublishRequest{RevisionID: "r1"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("denial and interruption return no findings", func(t *testing.T) {
		service, store, _, gateway := testService(t, workedPack(t))
		if _, err := service.Validate(context.Background(), Actor{"cloud", "stranger"}, "app", "r1"); !errors.Is(err, ErrAccessDenied) || len(store.lookups) != 0 {
			t.Fatalf("unauthorized selection lookup: %v", err)
		}
		if _, err := service.Publish(context.Background(), Actor{"cloud", "stranger"}, "app", PublishRequest{RevisionID: "r1"}); !errors.Is(err, ErrAccessDenied) || len(store.attempts)+len(store.lookups) != 0 {
			t.Fatalf("unauthorized publication lookup: %v", err)
		}
		gateway.onValidate = func(context.Context, engine.Projection, []byte, engineMetadata) (engineReport, *ValidationUnavailable) {
			return engineReport{Issues: []EngineIssue{{Severity: "ERROR", Message: "partial"}}}, &ValidationUnavailable{}
		}
		result, err := service.Validate(context.Background(), testOwner, "app", "r1")
		var unavailable *ValidationUnavailable
		if !errors.As(err, &unavailable) || !strings.Contains(err.Error(), "retry") ||
			result.RevisionID != "" || result.Valid || len(result.Issues)+len(result.CloudIssues)+len(result.PublicationBlockers) != 0 {
			t.Fatalf("interrupted validation returned %+v, %v", result, err)
		}
	})

	t.Run("completed errors preserve independent cloud findings", func(t *testing.T) {
		data := testPack(t, zip.Store,
			testZipMember{"sales/loomspan/execution.yaml", []byte("loomspan: {connections: {primary: {api-key: {$var: true}}}}")},
			testZipMember{"sales/loomspan/skills/read.yaml", []byte("executor: demo\nconfig: {path: {$var: lookup}}\n")},
		)
		service, _, _, gateway := testService(t, data)
		gateway.onValidate = func(context.Context, engine.Projection, []byte, engineMetadata) (engineReport, *ValidationUnavailable) {
			return engineReport{Issues: []EngineIssue{{Severity: "ERROR", SourceName: "loomspan/skills/read.yaml", Message: "Unknown executor"}}}, nil
		}
		result, err := service.Validate(context.Background(), testOwner, "app", "r1")
		if err != nil || result.Valid || len(result.Issues) != 1 || len(result.CloudIssues) != 1 || len(result.PublicationBlockers) != 1 {
			t.Fatalf("completed report %+v, %v", result, err)
		}
	})
}

func TestPublish(t *testing.T) {
	t.Run("first snapshot and stale staged confirmation", func(t *testing.T) {
		data := emptyPack(t, "sales")
		service, store, observer, gateway := testService(t, data)
		observer.set(testProjection(1, false))
		store.applications["app"].state.StagedRevision = "r2"
		ctx := context.Background()
		_, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r1"})
		failure := requirePublishFailure(t, err, PublishConfirmationRequired)
		if failure.Confirmation == nil || *failure.Confirmation != (StageConfirmation{"r1", "r2"}) || len(store.attempts) != 0 {
			t.Fatalf("unexpected staging confirmation: %+v", failure)
		}
		gateway.onInstall = func(_ context.Context, target engine.Projection, original []byte, meta engineMetadata) installResponse {
			if target.Ready || len(target.Namespaces) > 0 || meta.VariableValues == nil || len(*meta.VariableValues) > 0 ||
				meta.PublicationVersion == "" || !bytes.Equal(original, data) {
				t.Fatal("invalid first publish")
			}
			return installResponse{status: InstallAcknowledged}
		}
		got, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r1", Confirmation: failure.Confirmation})
		if err != nil || got.Version == "" || len(store.attempts) != 1 ||
			store.applications["app"].state.StagedRevision != "r2" {
			t.Fatalf("unexpected publish: %+v, %v", got, err)
		}
		take(t, service.refresh)
		if _, err := service.Eligible(ctx, "app"); !errors.Is(err, ErrPlacementUnavailable) {
			t.Fatal("ack routed before observation")
		}
		gateway.onInstall = nil
		already, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r1"})
		if err != nil || !already.AlreadyPublished || already.Message != "This revision is already published." ||
			already.Version != got.Version || len(store.attempts) != 1 {
			t.Fatalf("unexpected repeat result %+v, %v", already, err)
		}
	})

	t.Run("missing values and known unavailable states do not dispatch", func(t *testing.T) {
		service, store, observer, _ := testService(t, workedPack(t))
		ctx, req := context.Background(), PublishRequest{RevisionID: "r1"}
		_, err := service.Publish(ctx, testOwner, "app", req)
		if issue := requirePublishFailure(t, err, PublishBlocked); len(issue.Issues) != 6 {
			t.Fatalf("missing diagnostics: %+v", issue)
		}
		disconnected := testProjection(1, false)
		disconnected.Connected = false
		observer.set(disconnected)
		_, err = service.Publish(ctx, testOwner, "app", req)
		requirePublishFailure(t, err, PublishNotSent)
		terminating := testProjection(2, true)
		terminating.Terminating = true
		observer.set(terminating)
		_, err = service.Publish(ctx, testOwner, "app", req)
		if failure := requirePublishFailure(t, err, PublishNotSent); !strings.Contains(failure.Message, "shutting down") || len(store.attempts) != 0 {
			t.Fatalf("known termination: %+v", failure)
		}
	})

	t.Run("rejection and a lost response preserve previous publication", func(t *testing.T) {
		data := emptyPack(t, "sales")
		service, store, _, gateway := testService(t, data)
		app := store.applications["app"]
		app.state.Current = Publication{"app", "sales", "r2", "previous", app.revisions["r2"]}
		ctx, req := context.Background(), PublishRequest{RevisionID: "r1"}
		for _, tc := range []struct {
			response installResponse
			outcome  PublishOutcome
		}{
			{installResponse{status: InstallRejected, message: "Bad release café\n"}, PublishRejected},
			{installResponse{status: InstallUnconfirmed}, PublishUnconfirmed},
		} {
			gateway.onInstall = func(context.Context, engine.Projection, []byte, engineMetadata) installResponse { return tc.response }
			_, err := service.Publish(ctx, testOwner, "app", req)
			failure := requirePublishFailure(t, err, tc.outcome)
			if tc.response.message != "" && failure.Message != tc.response.message {
				t.Fatalf("engine text changed: %q", failure.Message)
			}
			if app.state.Current.Version != "previous" || len(service.refresh) != 0 {
				t.Fatal("unsuccessful request changed current publication")
			}
		}
		if !slices.Equal(store.finished, []InstallStatus{InstallRejected, InstallUnconfirmed}) || len(store.attempts) != 2 {
			t.Fatalf("unexpected recorded statuses: %v", store.finished)
		}
	})
}

func requirePublishFailure(t *testing.T, err error, outcome PublishOutcome) *PublishFailure {
	t.Helper()
	var failure *PublishFailure
	if !errors.As(err, &failure) || failure.Outcome != outcome {
		t.Fatalf("got %v, want publish outcome %s", err, outcome)
	}
	return failure
}
