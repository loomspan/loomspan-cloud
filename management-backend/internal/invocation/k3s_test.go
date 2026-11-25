//go:build k3s_integration

package invocation

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
	"github.com/loomspan/loomspan-cloud/management-backend/internal/publication"
)

const milestoneApp = "milestone"
const milestoneUser = "milestone-user"
const milestoneIssuer = "loomspan-cloud"
const milestoneSkill = "echo-release"
const milestonePackRoot = "/probe/packs"

func TestK3sRestoredSkill(t *testing.T) {
	if os.Getenv("LOOMSPAN_RUN_K3S_MILESTONE") != "1" {
		t.Skip("use the scoped disposable K3s milestone runner")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	db, dsn := milestoneDatabase(t, ctx)
	defer db.Close()
	seedMilestone(t, ctx, db)

	transport, err := engine.InClusterTransport()
	if err != nil {
		t.Fatal(err)
	}
	gates := newMilestoneTransport(transport)
	defer gates.releaseAll()
	console, err := engine.NewConsole(gates, os.Getenv("LOOMSPAN_CONSOLE_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	observer := engine.NewObserver(console, func(err error) { t.Logf("Console observation interrupted: %v", err) })
	watch, err := engine.InClusterPodWatch()
	if err != nil {
		t.Fatal(err)
	}
	pods := make(chan *engine.Pod)
	store, err := publication.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	packs, err := publication.OpenMountedPacks(milestonePackRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer packs.Close()
	owner := publication.Actor{Issuer: milestoneIssuer, Subject: milestoneUser}
	authorize := publication.AuthorizerFunc(func(_ context.Context, actor publication.Actor, app string, grant publication.Permission) error {
		if actor != owner || app != milestoneApp || (grant != publication.PermissionPublish && grant != publication.PermissionValidate) {
			return publication.ErrAccessDenied
		}
		return nil
	})
	publishing := publication.NewService(store, packs, observer, publication.NewEngineClient(observer, gates), authorize)
	publishing.SetRecoveryReporter(func(failure publication.RecoveryFailure) { t.Logf("recovery failure: %+v", failure) })
	var workers sync.WaitGroup
	workers.Go(func() { watch.Run(ctx, pods, func(err error) { t.Logf("pod watch interrupted: %v", err) }) })
	workers.Go(func() { observer.Run(ctx, pods) })
	workers.Go(func() { publishing.Run(ctx) })
	defer func() { cancel(); workers.Wait() }()

	executor, err := NewService(NewPostgresGrants(db), publishing, gates, milestoneIssuer)
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{Kind: DirectUser, ID: milestoneUser}
	old := observeUntil(t, ctx, observer, "first authenticated snapshot", func(p engine.Projection) bool {
		return p.Connected
	})
	requireNoCloudPublications(t, old)
	first, err := publishing.Publish(ctx, owner, milestoneApp, publication.PublishRequest{RevisionID: "first"})
	if err != nil || first.Version == "" {
		t.Fatalf("first publication after initial snapshot: %+v, %v", first, err)
	}
	observedFirst := observeUntil(t, ctx, observer, "automatic refresh for first publication", func(p engine.Projection) bool {
		return p.UID == old.UID && p.InstanceID == old.InstanceID && observedVersion(p, first.Version)
	})
	if row := observedFirst.Namespaces[milestoneApp]; row.TracePersistencePolicy != "NEVER" {
		t.Fatalf("namespace trace policy = %q, want NEVER", row.TracePersistencePolicy)
	}
	gates.requireAcknowledgment(t, old.UID, first.Version)
	t.Logf("first acknowledgment refreshed pod=%s uid=%s jvm=%s publication=%s",
		old.Name, old.UID, old.InstanceID, first.Version)

	setStage(t, ctx, db, "invalid")
	_, err = publishing.Publish(ctx, owner, milestoneApp, publication.PublishRequest{RevisionID: "invalid"})
	var failed *publication.PublishFailure
	if !errors.As(err, &failed) || failed.Outcome != publication.PublishRejected {
		t.Fatalf("engine did not reject the invalid replacement: %v", err)
	}
	current, err := store.CurrentPublication(ctx, milestoneApp)
	if err != nil || current.Version != first.Version {
		t.Fatalf("invalid replacement changed the pointer: %+v, %v", current, err)
	}
	fresh, err := observer.Refresh(ctx)
	if err != nil || fresh.UID != old.UID || fresh.InstanceID != old.InstanceID || !observedVersion(fresh, first.Version) {
		t.Fatalf("invalid replacement changed the original serving generation: %v", err)
	}

	oldGate := gates.holdSnapshots(old.UID)
	setStage(t, ctx, db, "restored")
	restored, err := publishing.Publish(ctx, owner, milestoneApp, publication.PublishRequest{RevisionID: "restored"})
	if err != nil || restored.Version == "" || restored.Version == first.Version {
		t.Fatalf("newer publication: %+v, %v", restored, err)
	}
	gates.requireAcknowledgment(t, old.UID, restored.Version)
	reopened, err := publication.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	recovered, reopenErr := reopened.CurrentPublication(ctx, milestoneApp)
	reopened.Close()
	if reopenErr != nil || recovered.Version != restored.Version || recovered.RevisionID != "restored" ||
		recovered.PackDigest != stageDigest(t, ctx, db, "restored") {
		t.Fatalf("acknowledged pointer does not reopen: %+v, %v", recovered, reopenErr)
	}
	waitSignal(t, ctx, oldGate.attempted, "old pod's requested automatic snapshot refresh")
	if !observedVersion(observer.Current(), first.Version) {
		t.Fatal("newer version was exposed before the old snapshot barrier")
	}
	requireUnavailable(t, ctx, executor, actor, gates)
	gates.holdReplacementOnPublish(old.UID, restored.Version)
	fmt.Printf("K3S_MILESTONE_REPLACE %s %s %s %s\n", old.Name, old.UID, old.InstanceID, restored.Version)

	replacement := observeUntil(t, ctx, observer, "replacement JVM's initially empty snapshot", func(p engine.Projection) bool {
		return p.Connected && p.UID != old.UID && p.InstanceID != old.InstanceID
	})
	requireNoCloudPublications(t, replacement)
	waitUntil(t, ctx, "acknowledged restore persisted on replacement", func() bool {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM loomspan_cloud.install_attempts
			WHERE application_id = $1 AND version = $2 AND pod_uid = $3 AND instance_id = $4
			AND purpose = 'restore' AND status = 'acknowledged'`,
			milestoneApp, restored.Version, replacement.UID, replacement.InstanceID).Scan(&count)
		return err == nil && count == 1
	})
	gates.requireAcknowledgment(t, replacement.UID, restored.Version)
	replacementGate := gates.snapshotGate(replacement.UID)
	if replacementGate == nil {
		t.Fatal("the replacement snapshot was not gated at restore dispatch")
	}
	waitSignal(t, ctx, replacementGate.attempted, "replacement refresh attempted before confirmation")
	requireUnavailable(t, ctx, executor, actor, gates)
	replacementGate.release()
	confirmed := observeUntil(t, ctx, observer, "replacement snapshot confirms restored version", func(p engine.Projection) bool {
		row, ok := p.Confirms(milestoneApp, restored.Version)
		return ok && row.TracePersistencePolicy == "NEVER" && p.UID == replacement.UID &&
			p.InstanceID == replacement.InstanceID
	})

	result := invokeMilestone(t, ctx, executor, actor, `{"input":{"text":"ping"}}`, http.StatusOK)
	body, err := json.Marshal(result.Body)
	if err != nil || string(body) != `{"result":"restored/ping"}` {
		t.Fatalf("first eligible exact response = %s, %v", body, err)
	}
	invocations := gates.invocations()
	if len(invocations) != 1 || invocations[0].uid != confirmed.UID || invocations[0].request.ExecutionView != "" ||
		invocations[0].request.Skill != milestoneSkill || string(invocations[0].request.Input) != `{"text":"ping"}` {
		t.Fatalf("first invocation did not use the confirmed replacement: %+v", invocations)
	}
	fmt.Printf("K3S_MILESTONE_RESTORED oldJvm=%s newJvm=%s publication=%s prematureInvocations=0 result=%s\n",
		old.InstanceID, confirmed.InstanceID, restored.Version, body)
	assertMilestoneDiagnostics(t, ctx, db, executor, actor, gates)
	testPostgresGrantIsolation(t, ctx, db)
}

func milestoneDatabase(t *testing.T, ctx context.Context) (*sql.DB, string) {
	t.Helper()
	password := os.Getenv("LOOMSPAN_MILESTONE_DB_PASSWORD")
	if password == "" {
		t.Fatal("fixture database password missing")
	}
	address := url.URL{Scheme: "postgres", User: url.UserPassword("loomspan", password),
		Host: "loomspan-milestone-postgres:5432", Path: "/milestone", RawQuery: "sslmode=disable"}
	db, err := sql.Open("pgx", address.String())
	if err != nil {
		t.Fatal("fixture database configuration failed")
	}
	waitUntil(t, ctx, "fixture PostgreSQL available", func() bool { return db.PingContext(ctx) == nil })
	for _, name := range []string{"001_publications.sql", "002_invocation.sql"} {
		data, err := os.ReadFile(filepath.Join("/probe/migrations", name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx, string(data)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return db, address.String()
}

func seedMilestone(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	execFixtureSQL(t, ctx, db, `INSERT INTO loomspan_cloud.applications (id, namespace, trace_persistence)
		VALUES ($1, $1, 'NEVER')`, milestoneApp)
	execFixtureSQL(t, ctx, db, `INSERT INTO loomspan_cloud.application_invoke_users (application_id, subject, roles)
		VALUES ($1, $2, ARRAY['reviewer'])`, milestoneApp, milestoneUser)
	for _, revision := range []string{"first", "invalid", "restored"} {
		pack := milestonePack(t, revision)
		digest := fmt.Sprintf("%x", sha256.Sum256(pack))
		if err := os.WriteFile(filepath.Join(milestonePackRoot, digest+".zip"), pack, 0600); err != nil {
			t.Fatal(err)
		}
		execFixtureSQL(t, ctx, db, `INSERT INTO loomspan_cloud.stage_revisions
			(application_id, revision_id, pack_digest) VALUES ($1,$2,$3)`, milestoneApp, revision, digest)
	}
	setStage(t, ctx, db, "first")
}

func milestonePack(t *testing.T, revision string) []byte {
	t.Helper()
	var pack bytes.Buffer
	zipper := zip.NewWriter(&pack)
	executor := "milestone-host:echo-release"
	if revision == "invalid" {
		executor = "milestone-host:does-not-exist"
	}
	skill := fmt.Sprintf(`name: echo-release
description: Echo release label and input
executor: %s
rbac_roles: [reviewer]
input_schema:
  type: object
  required: [text]
  properties:
    text: {type: string}
config: {release: %s}
`, executor, revision)
	for _, file := range []struct{ name, data string }{
		{"milestone/loomspan/execution.yaml", "loomspan: {}\n"},
		{"milestone/loomspan/skills/echo-release.yaml", skill},
	} {
		out, err := zipper.Create(file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write([]byte(file.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipper.Close(); err != nil {
		t.Fatal(err)
	}
	return pack.Bytes()
}

func execFixtureSQL(t *testing.T, ctx context.Context, db *sql.DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, sql, args...); err != nil {
		t.Fatalf("fixture SQL failed: %v", err)
	}
}

func setStage(t *testing.T, ctx context.Context, db *sql.DB, revision string) {
	t.Helper()
	execFixtureSQL(t, ctx, db, `UPDATE loomspan_cloud.applications SET current_revision_id = $1 WHERE id = $2`,
		revision, milestoneApp)
}

func stageDigest(t *testing.T, ctx context.Context, db *sql.DB, revision string) string {
	t.Helper()
	var digest string
	if err := db.QueryRowContext(ctx, `SELECT pack_digest FROM loomspan_cloud.stage_revisions
		WHERE application_id = $1 AND revision_id = $2`, milestoneApp, revision).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	return digest
}

func observeUntil(t *testing.T, ctx context.Context, observer *engine.Observer, description string,
	match func(engine.Projection) bool) engine.Projection {
	t.Helper()
	for {
		update := observer.Changes()
		snapshot := observer.Current()
		if match(snapshot) {
			return snapshot
		}
		select {
		case <-update:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s, last observation: %+v", description, snapshot)
		}
	}
}

func observedVersion(p engine.Projection, version string) bool {
	row, exists := p.Namespaces[milestoneApp]
	return p.Connected && exists && row.PublicationVersion != nil && *row.PublicationVersion == version
}

func requireNoCloudPublications(t *testing.T, p engine.Projection) {
	t.Helper()
	for name, row := range p.Namespaces {
		if name == milestoneApp || row.PublicationVersion != nil {
			t.Fatalf("Cloud namespace unexpectedly present before publication: %q", name)
		}
	}
}

func waitSignal(t *testing.T, ctx context.Context, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitUntil(t *testing.T, ctx context.Context, description string, match func() bool) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for !match() {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", description)
		}
	}
}

func invokeMilestone(t *testing.T, ctx context.Context, service *Service, actor Actor, body string, status int) Response {
	t.Helper()
	result, err := service.InvokeJSON(ctx, actor, milestoneApp, milestoneSkill, strings.NewReader(body))
	if err != nil || result.StatusCode != status {
		t.Fatalf("invoke %s: status=%d, message=%s, error=%v", body, result.StatusCode, result.Body.Message, err)
	}
	return result
}

func requireUnavailable(t *testing.T, ctx context.Context, service *Service, actor Actor, gates *milestoneTransport) {
	t.Helper()
	_, err := service.InvokeJSON(ctx, actor, milestoneApp, milestoneSkill, strings.NewReader(`{"input":{"text":"ping"}}`))
	if err != ErrUnavailable || len(gates.invocations()) != 0 {
		t.Fatalf("invocation reached engine before confirmed eligibility, err=%v dispatched=%d", err, len(gates.invocations()))
	}
}

func assertMilestoneDiagnostics(t *testing.T, ctx context.Context, db *sql.DB, service *Service, actor Actor, gates *milestoneTransport) {
	t.Helper()
	for _, enabled := range []bool{false, true} {
		execFixtureSQL(t, ctx, db, `UPDATE loomspan_cloud.applications SET show_execution_details=$1 WHERE id=$2`,
			enabled, milestoneApp)
		for _, input := range []string{"ping", "fail"} {
			expected := http.StatusOK
			if input == "fail" {
				expected = http.StatusInternalServerError
			}
			result := invokeMilestone(t, ctx, service, actor, `{"input":{"text":"`+input+`"},"executionView":"always"}`, expected)
			captures := gates.invocations()
			last := captures[len(captures)-1]
			if (last.request.ExecutionView == "always") != enabled || (result.Body.ExecutionView != nil) != enabled ||
				last.request.Caller.Issuer != milestoneIssuer || last.request.Caller.Subject != milestoneUser ||
				len(last.request.Caller.Roles) != 1 || last.request.Caller.Roles[0] != "reviewer" {
				t.Fatalf("wrong mode or caller on %s with enabled=%v", input, enabled)
			}
			if enabled && (!bytes.Equal(result.Body.ExecutionView, last.response["executionView"]) ||
				!bytes.Contains(result.Body.ExecutionView, []byte(`"sessionId"`)) ||
				!bytes.Contains(result.Body.ExecutionView, []byte(`"events"`))) {
				t.Fatal("the complete Framework execution view was lost")
			}
			if input == "ping" {
				if result.Body.Result == nil || *result.Body.Result != "restored/ping" {
					t.Fatalf("incorrect execution result: %+v", result)
				}
			} else if !bytes.Equal(result.Body.Message, last.response["message"]) ||
				!bytes.Contains(last.response["debug"], []byte("Milestone executor received fail")) ||
				(enabled && (!bytes.Equal(result.Body.Debug, last.response["debug"]) ||
					!bytes.Equal(result.Body.Diagnosis, last.response["diagnosis"]))) ||
				(!enabled && (result.Body.Debug != nil || result.Body.Diagnosis != nil)) {
				t.Fatal("execution failure response changed when applying view policy")
			}
		}
	}
	fallback := invokeMilestone(t, ctx, service, actor, `{"input":{}}`, http.StatusBadRequest)
	if string(fallback.Body.ExecutionView) != "null" || !bytes.Contains(fallback.Body.Issues, []byte("required")) ||
		!bytes.Contains(fallback.Body.Diagnosis, []byte("required")) {
		t.Fatalf("pre-session null-view fallback lost validation evidence: %+v", fallback)
	}
	fmt.Println("K3S_MILESTONE_DIAGNOSTICS off/on success/failure, real failure and pre-session null view verified under NEVER")
}

func testPostgresGrantIsolation(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	execFixtureSQL(t, ctx, db, `INSERT INTO loomspan_cloud.applications(id, namespace) VALUES ('another', 'another')`)
	execFixtureSQL(t, ctx, db, `INSERT INTO loomspan_cloud.application_invoke_backends(application_id, backend_id)
		VALUES ('another', 'allowed-backend')`)
	grants := NewPostgresGrants(db)
	backend := Actor{Kind: CallingBackend, ID: "allowed-backend"}
	grant, err := grants.Lookup(ctx, "another", backend)
	if err != nil || grant.ShowExecution || grant.Namespace != "another" {
		t.Fatalf("backend grant or default diagnostic switch: %+v, %v", grant, err)
	}
	if _, err := grants.Lookup(ctx, milestoneApp, backend); err != ErrAccessDenied {
		t.Fatalf("backend grant escaped application scope: %v", err)
	}
	if _, err := grants.Lookup(ctx, "another", Actor{Kind: DirectUser, ID: milestoneUser}); err != ErrAccessDenied {
		t.Fatalf("signed-in user grant escaped application scope: %v", err)
	}
}
