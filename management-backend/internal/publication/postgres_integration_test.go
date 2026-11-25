package publication

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

type completeHookStore struct {
	Store
	before func(*InstallAttempt, InstallStatus)
}

func (s completeHookStore) CompleteInstall(ctx context.Context, attempt InstallAttempt, status InstallStatus) error {
	s.before(&attempt, status)
	return s.Store.CompleteInstall(ctx, attempt, status)
}

func TestPublicationPostgres(t *testing.T) {
	dsn := os.Getenv("LOOMSPAN_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("disposable PostgreSQL not configured")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" || parsed.Path != "/loomspan_pr14" ||
		net.ParseIP(parsed.Hostname()) == nil || !net.ParseIP(parsed.Hostname()).IsLoopback() {
		t.Fatal("integration requires the disposable loopback PostgreSQL database from the test plan")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	setup, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("cannot configure the disposable database")
	}
	defer setup.Close()
	migration := readFixture(t, "../../migrations/001_publications.sql")
	execSQL(t, setup, ctx, string(migration))
	store, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dir := t.TempDir()
	packs, err := OpenMountedPacks(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer packs.Close()
	authored := workedPack(t)
	digest := testWritePack(t, dir, authored)
	execSQL(t, setup, ctx, `
		INSERT INTO loomspan_cloud.applications (id, namespace, trace_persistence)
		VALUES ('app', 'sales', 'NEVER'), ('other', 'other', 'ONERROR')
	`)
	execSQL(t, setup, ctx, `
		INSERT INTO loomspan_cloud.stage_revisions (application_id, revision_id, pack_digest)
		VALUES ('app','r1',$1), ('app','r2',$1), ('app','r3',$1), ('other','private',$1)
	`, digest)
	execSQL(t, setup, ctx, "UPDATE loomspan_cloud.applications SET current_revision_id = 'r1' WHERE id = 'app'")
	execSQL(t, setup, ctx, `
		INSERT INTO loomspan_cloud.app_variables (application_id, name, value) VALUES
		('app','provider.key','mine'), ('other','provider.key','theirs'), ('app','app.region','west'),
		('app','records.url','https://records.example.test'), ('app','records.lookup-path','path'), ('app','unused','other value')
	`)
	selected, err := store.OpenSelection(ctx, SelectionQuery{"app", "r1"}, func(Selection) ([]string, error) {
		return []string{"provider.key"}, nil
	})
	if err != nil || !maps.Equal(selected.SavedValues, map[string]string{"provider.key": "mine"}) {
		t.Fatalf("unexpected scoping of saved values: %v", err)
	}
	if _, err := setup.ExecContext(ctx, "UPDATE loomspan_cloud.applications SET current_revision_id = 'private' WHERE id = 'app'"); err == nil {
		t.Fatal("a cross-application staged pointer was accepted")
	}
	if _, err := store.OpenSelection(ctx, SelectionQuery{"app", "private"}, func(Selection) ([]string, error) {
		t.Fatal("cross-application revision was exposed")
		return nil, nil
	}); !errors.Is(err, ErrSelectionNotFound) {
		t.Fatal("cross-application revision did not report not found")
	}

	observer := newFakeObserver(testProjection(1, true))
	gateway := &fakeGateway{
		onInstall: func(context.Context, engine.Projection, []byte, engineMetadata) installResponse {
			return installResponse{status: InstallAcknowledged}
		},
	}
	service := newService(store, packs, observer, gateway, AuthorizerFunc(authorizeOwner))
	first, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setup.ExecContext(ctx, "UPDATE loomspan_cloud.applications SET current_publication_version = $1 WHERE id = 'other'", first.Version); err == nil {
		t.Fatal("a cross-application publication pointer was accepted")
	}
	execSQL(t, setup, ctx, "UPDATE loomspan_cloud.applications SET current_revision_id = 'r2' WHERE id = 'app'")

	entered := make(chan engineMetadata, 1)
	acknowledge := make(chan struct{})
	gateway.onInstall = func(_ context.Context, _ engine.Projection, pack []byte, metadata engineMetadata) installResponse {
		if !bytes.Equal(pack, authored) {
			t.Error("publication did not send immutable original ZIP")
		}
		entered <- metadata
		<-acknowledge
		return installResponse{status: InstallAcknowledged}
	}
	startedWrite := make(chan struct{}, 1)
	service.store = completeHookStore{Store: store, before: func(*InstallAttempt, InstallStatus) { startedWrite <- struct{}{} }}
	type publishOutcome struct {
		result PublishResult
		err    error
	}
	finished := make(chan publishOutcome, 1)
	go func() {
		result, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r2"})
		finished <- publishOutcome{result, err}
	}()
	meta := take(t, entered)
	t.Run("acknowledgment after the durable transaction", func(t *testing.T) {
		lock, err := setup.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Rollback()
		execSQL(t, lock, ctx, "SELECT id FROM loomspan_cloud.applications WHERE id = 'app' FOR NO KEY UPDATE")
		execSQL(t, lock, ctx, "UPDATE loomspan_cloud.applications SET trace_persistence = 'ALWAYS' WHERE id = 'app'")
		close(acknowledge)
		take(t, startedWrite)
		previous, err := store.CurrentPublication(ctx, "app")
		if err != nil || previous.Version != first.Version {
			t.Fatalf("pointer advanced before commit: %+v %v", previous, err)
		}
		select {
		case got := <-finished:
			t.Fatalf("returned before durable acknowledgment: %+v", got)
		default:
		}
		if err := lock.Commit(); err != nil {
			t.Fatal(err)
		}
	})
	out := take(t, finished)
	if out.err != nil || out.result.Version != meta.PublicationVersion {
		t.Fatalf("acknowledged publication failed: %v", out.err)
	}
	reopened, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	current, err := reopened.CurrentPublication(ctx, "app")
	if err != nil || current.PackDigest != digest || current.Version != meta.PublicationVersion || current.RevisionID != "r2" {
		t.Fatalf("durable pointer = %+v, %v", current, err)
	}
	var sentJSON []byte
	var saved string
	err = setup.QueryRowContext(ctx, `
		SELECT i.settings, a.trace_persistence FROM loomspan_cloud.install_attempts i
		JOIN loomspan_cloud.applications a ON a.id = i.application_id
		WHERE i.version = $1 AND i.status = 'acknowledged' AND i.purpose = 'publish'
	`, current.Version).Scan(&sentJSON, &saved)
	var sent RuntimeSettings
	if err != nil || json.Unmarshal(sentJSON, &sent) != nil || saved != TraceAlways ||
		sent.ExecutionTrace.Persistence != TraceNever || sent != meta.RuntimeSettings {
		t.Fatalf("saved policy and sent policy were not independent: %v", err)
	}

	execSQL(t, setup, ctx, "UPDATE loomspan_cloud.applications SET current_revision_id = 'r3' WHERE id = 'app'")
	service.store = store
	for _, response := range []installResponse{
		{status: InstallRejected, message: "invalid release\n"},
		{status: InstallUnconfirmed},
		{status: InstallNotSent},
	} {
		gateway.onInstall = func(context.Context, engine.Projection, []byte, engineMetadata) installResponse { return response }
		_, err := service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r3"})
		requirePublishFailure(t, err, PublishOutcome(response.status))
	}
	states, err := setup.QueryContext(ctx, `
		SELECT i.status FROM loomspan_cloud.install_attempts i JOIN loomspan_cloud.publications p ON p.version = i.version
		WHERE p.revision_id = 'r3' AND p.status = i.status ORDER BY i.status
	`)
	if err != nil {
		t.Fatal(err)
	}
	var recorded []string
	for states.Next() {
		var status string
		if err := states.Scan(&status); err != nil {
			t.Fatal(err)
		}
		recorded = append(recorded, status)
	}
	if err := states.Close(); err != nil || !slices.Equal(recorded, []string{"not-sent", "rejected", "unconfirmed"}) {
		t.Fatalf("unacknowledged states: %v, %v", recorded, err)
	}

	service.store = completeHookStore{Store: store, before: func(attempt *InstallAttempt, _ InstallStatus) {
		attempt.ExpectedVersion = "stale-pointer"
	}}
	gateway.onInstall = func(context.Context, engine.Projection, []byte, engineMetadata) installResponse {
		return installResponse{status: InstallAcknowledged}
	}
	_, err = service.Publish(ctx, testOwner, "app", PublishRequest{RevisionID: "r3"})
	failure := requirePublishFailure(t, err, PublishAcknowledgmentError)
	var attemptStatus, publicationStatus string
	err = setup.QueryRowContext(ctx, `
		SELECT i.status, p.status FROM loomspan_cloud.install_attempts i
		JOIN loomspan_cloud.publications p ON p.version = i.version WHERE p.version = $1
	`, failure.Version).Scan(&attemptStatus, &publicationStatus)
	if err != nil || attemptStatus != "started" || publicationStatus != "started" {
		t.Fatalf("failed acknowledgment left a partial transaction: %s %s %v", attemptStatus, publicationStatus, err)
	}
	pointer, err := reopened.CurrentPublication(ctx, "app")
	if err != nil || pointer != current {
		t.Fatalf("unsuccessful installs changed the durable pointer: %+v, %v", pointer, err)
	}

	execSQL(t, setup, ctx, "UPDATE loomspan_cloud.app_variables SET value = 'changed' WHERE application_id = 'app' AND name = 'provider.key'")
	replacement := testProjection(2, true)
	replacement.InstanceID, replacement.UID = "replacement-jvm", "replacement-pod"
	newObserver := newFakeObserver(replacement)
	recovering := newService(reopened, packs, newObserver, gateway, nil)
	gateway.onInstall = func(_ context.Context, target engine.Projection, pack []byte, meta engineMetadata) installResponse {
		if target.InstanceID != "replacement-jvm" || meta.PublicationVersion != current.Version ||
			meta.RuntimeSettings.ExecutionTrace.Persistence != TraceAlways || !bytes.Equal(pack, authored) ||
			(*meta.VariableValues)["provider.key"] != "changed" {
			t.Error("restore did not use the stored ZIP/version and current saved inputs")
		}
		return installResponse{status: InstallAcknowledged}
	}
	if err := recovering.reconcile(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	take(t, recovering.refresh)
	if _, err := recovering.Eligible(ctx, "app"); !errors.Is(err, ErrPlacementUnavailable) {
		t.Fatal("restoration routed before the new observed tuple")
	}
	markInstalled(&replacement, "sales", current.Version, "restored", TraceAlways)
	newObserver.set(replacement)
	placement, err := recovering.Eligible(ctx, "app")
	if err != nil || placement.GenerationID != "restored" || placement.Version != current.Version {
		t.Fatalf("restored version unavailable: %+v, %v", placement, err)
	}
	var restoreStatus string
	err = setup.QueryRowContext(ctx, "SELECT status FROM loomspan_cloud.install_attempts WHERE version = $1 AND purpose = 'restore'", current.Version).Scan(&restoreStatus)
	if err != nil || restoreStatus != "acknowledged" {
		t.Fatalf("restore acknowledgment not recorded: %s %v", restoreStatus, err)
	}
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func execSQL(t *testing.T, executor sqlExecutor, ctx context.Context, query string, args ...any) {
	t.Helper()
	if _, err := executor.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("fixture SQL failed: %s: %v", strings.TrimSpace(query), err)
	}
}
