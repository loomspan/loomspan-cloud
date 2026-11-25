//go:build k3s_integration

package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	probeNamespace = "observation-probe"
	probeVersion   = "probe-version-1"
)

func TestK3sEngineObservation(t *testing.T) {
	key := os.Getenv("LOOMSPAN_CONSOLE_API_KEY")
	if key == "" {
		t.Skip("run from the opt-in observation Job with its mounted credentials")
	}
	watch, err := InClusterPodWatch()
	if err != nil {
		t.Fatal(err)
	}
	transport, err := InClusterTransport()
	if err != nil {
		t.Fatal(err)
	}
	console, err := NewConsole(transport, key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 210*time.Second)
	observer := NewObserver(console, func(err error) { t.Log(err) })
	pods := make(chan *Pod)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()
	workers.Go(func() { watch.Run(ctx, pods, func(err error) { t.Log(err) }) })
	workers.Go(func() { observer.Run(ctx, pods) })

	first := awaitProjection(t, ctx, observer, func(p Projection) bool { return p.Connected })
	for _, row := range first.Namespaces {
		if row.PublicationVersion != nil {
			t.Fatalf("Cloud application %s was installed before the first snapshot", row.Namespace)
		}
	}
	pod, eligible := first.PublicationTarget()
	if !eligible || pod.Namespace != EngineNamespace {
		t.Fatalf("first authenticated snapshot is not usable: %+v", first)
	}
	t.Logf("FIRST_SNAPSHOT pod=%s uid=%s instance=%s ready=%t", pod.Name, pod.UID, first.InstanceID, pod.Ready)

	badConsole, err := NewConsole(transport, otherTestKey(key))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := badConsole.connect(pod)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.client.CloseIdleConnections()
	if _, err = connection.snapshot(ctx); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("an authenticated client with a wrong Console key must receive 401: %v", err)
	}

	publishProbe(t, ctx, transport, pod)
	fresh, err := observer.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row := fresh.Namespaces[probeNamespace]
	if fresh.InstanceID != first.InstanceID || row.GenerationID == "" || row.PublicationVersion == nil ||
		*row.PublicationVersion != probeVersion || row.TracePersistencePolicy != "NEVER" ||
		fresh.TracePersistencePolicy != "ONERROR" {
		t.Fatalf("snapshot did not bind the publication policy to its generation: %+v", fresh)
	}
	confirmed := awaitProjection(t, ctx, observer, func(p Projection) bool {
		observed, eligible := p.Confirms(probeNamespace, probeVersion)
		return p.ActivityConnected && eligible && p.UID == first.UID && p.InstanceID == first.InstanceID &&
			observed.GenerationID == row.GenerationID && observed.TracePersistencePolicy == row.TracePersistencePolicy
	})
	t.Logf("READY_FOR_REPLACEMENT pod=%s uid=%s instance=%s generation=%s publication=%s policy=%s ready=%t",
		confirmed.Name, confirmed.UID, confirmed.InstanceID, row.GenerationID,
		*row.PublicationVersion, row.TracePersistencePolicy, confirmed.Ready)

	awaitProjection(t, ctx, observer, func(p Projection) bool {
		_, eligible := p.Confirms(probeNamespace, probeVersion)
		return !eligible && (!p.Connected || p.Pod.Terminating || p.UID != first.UID)
	})
	next := awaitProjection(t, ctx, observer, func(p Projection) bool {
		return p.ActivityConnected && p.UID != first.UID && p.InstanceID != first.InstanceID
	})
	if _, exists := next.Namespaces[probeNamespace]; exists || next.ConnectionID <= first.ConnectionID {
		t.Fatalf("replacement inherited stale observation: %+v", next)
	}
	t.Logf("REPLACEMENT_SNAPSHOT pod=%s uid=%s instance=%s", next.Name, next.UID, next.InstanceID)
}

// TestK3sEngineNetworkPeer runs with no cluster or engine credentials. Its labels and namespace
// are set by the disposable-cluster verification command.
func TestK3sEngineNetworkPeer(t *testing.T) {
	address := os.Getenv("LOOMSPAN_K3S_ENGINE_IP")
	if address == "" {
		t.Skip("supply the watched test engine pod IP")
	}
	blocked := os.Getenv("LOOMSPAN_K3S_EXPECT_BLOCKED") == "true"
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	dialer := &net.Dialer{Timeout: time.Second}
	for ctx.Err() == nil {
		connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, "8443"))
		if err == nil {
			connection.Close()
			if blocked {
				t.Fatal("unrelated pod reached the engine API port")
			}
			t.Log("allowed management pod opened engine API TCP connection")
			return
		}
		if !retry(ctx, 200*time.Millisecond) {
			break
		}
	}
	if !blocked {
		t.Fatal("allowed management pod could not open TCP after NetworkPolicy reconciliation")
	}
	t.Log("unrelated pod remained unable to open engine API TCP after NetworkPolicy reconciliation")
}

func otherTestKey(key string) string {
	if key[0] == 'A' {
		return "B" + key[1:]
	}
	return "A" + key[1:]
}

func publishProbe(t *testing.T, ctx context.Context, transport *Transport, pod Pod) {
	t.Helper()
	var pack bytes.Buffer
	archive := zip.NewWriter(&pack)
	execution, err := archive.Create(probeNamespace + "/loomspan/execution.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(execution, "loomspan: {}\n"); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	packPart, err := form.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="pack"; filename="observation-probe.zip"`},
		"Content-Type":        {"application/zip"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pack.WriteTo(packPart); err != nil {
		t.Fatal(err)
	}
	metadataPart, err := form.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="metadata"`},
		"Content-Type":        {"application/json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(metadataPart, `{
  "namespace": %q,
  "publicationVersion": %q,
  "variableValues": {},
  "runtimeSettings": {
    "session": {
      "mission-timeout": "60s",
      "max-depth": 32,
      "attachments": { "max-size": "20971520B" },
      "quotas": {
        "max-skill-invocations": 64,
        "max-tool-invocations": 128,
        "max-linter-retries": 32,
        "max-model-calls": 64,
        "max-provider-attempts": 192,
        "max-usage-units": 200000
      }
    },
    "execution-trace": { "persistence": "NEVER" }
  }
}`, probeNamespace, probeVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	client, base, err := transport.ForPod(pod)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.JoinPath("/internal/v1/publish").String(), &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		details, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Fatalf("fixture publish returned HTTP %d with message %q", response.StatusCode, details)
	}
}
