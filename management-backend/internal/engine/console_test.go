package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	instanceOne = "d1b5ddcf-86c5-4302-80c9-3c0704e76b2f"
	instanceTwo = "2bc6d5c5-2d99-46ae-b855-7ec3809dc0df"
)

var testConsoleKey = strings.Repeat("K", 32)

func TestConsoleReadsSnapshotBeforeStream(t *testing.T) {
	var order []string
	var orderMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(consoleKey) != testConsoleKey {
			t.Error("Console request omitted its API key")
		}
		name := strings.TrimPrefix(r.URL.Path, consolePath)
		orderMu.Lock()
		order = append(order, name)
		orderMu.Unlock()
		switch name {
		case "/instance":
			writeSnapshot(w, instanceOne, "ONERROR", []NamespaceObservation{})
		case "/active-executions":
			if r.URL.Query().Get("pageSize") != "1" {
				t.Error("baseline did not use a small page")
			}
			writeBaseline(w, instanceOne)
		case "/activity":
			requireActivityRequest(t, r, instanceOne)
			writeHandshake(w, instanceOne)
			_, _ = fmt.Fprintf(w, "id: 9\nevent: activity\ndata: {\"instanceId\":%q,\"cursor\":\"9\"}\n\n", instanceOne)
		default:
			t.Error("unexpected Console path")
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	conn, err := testConsole(t, server).connect(Pod{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := conn.snapshot(t.Context())
	if err != nil || snapshot.InstanceID != instanceOne || len(snapshot.Namespaces) != 0 {
		t.Fatalf("valid empty Console snapshot: %#v, %v", snapshot, err)
	}
	admitted := false
	err = conn.stream(t.Context(), snapshot.InstanceID, func() { admitted = true })
	if err == nil || !admitted {
		t.Fatalf("stream should admit and then observe EOF: admitted=%v, error=%v", admitted, err)
	}
	orderMu.Lock()
	defer orderMu.Unlock()
	if !slices.Equal(order, []string{"/instance", "/active-executions", "/activity"}) {
		t.Fatalf("unexpected Console request order: %v", order)
	}
}

func TestConsoleRejectsMismatchedIdentityOrGeneration(t *testing.T) {
	published := NamespaceObservation{"app", "app-1", new("release"), "NEVER"}
	for _, test := range []struct {
		name string
		id   string
		rows []NamespaceObservation
	}{
		{"mismatched-header", instanceTwo, []NamespaceObservation{published}},
		{"absent-namespaces", instanceOne, nil},
		{"duplicate-namespace", instanceOne, []NamespaceObservation{published, published}},
		{"missing-generation", instanceOne, []NamespaceObservation{{"app", "", new("release"), "NEVER"}}},
		{"missing-namespace-policy", instanceOne, []NamespaceObservation{{"app", "app-1", new("release"), ""}}},
		{"invalid-namespace-policy", instanceOne, []NamespaceObservation{{"app", "app-1", new("release"), "maybe"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(instanceHeader, test.id)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(instanceBody{instanceOne, "ONERROR", test.rows})
			}))
			defer server.Close()
			conn, err := testConsole(t, server).connect(Pod{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := conn.snapshot(t.Context()); err == nil {
				t.Fatal("invalid snapshot was accepted")
			}
		})
	}
}

func TestConsolePreservesOpaquePublicationVersion(t *testing.T) {
	const exactVersion = " release\t"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSnapshot(w, instanceOne, "ONERROR",
			[]NamespaceObservation{{"app", "app-1", new(exactVersion), "NEVER"}})
	}))
	defer server.Close()
	conn, err := testConsole(t, server).connect(Pod{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := conn.snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if version := snapshot.Namespaces["app"].PublicationVersion; version == nil || *version != exactVersion {
		t.Fatalf("publication version changed: %v", version)
	}
}

func TestConsoleRejectsBrokenActivity(t *testing.T) {
	for _, test := range []struct {
		name, scenario string
		admitted       bool
	}{
		{"mismatched-instance", "identity", false},
		{"evicted-cursor", "gone", false},
		{"invalid-activity", "malformed", true},
		{"oversized-frame", "oversized", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/active-executions") {
					writeBaseline(w, instanceOne)
					return
				}
				switch test.scenario {
				case "identity":
					writeHandshake(w, instanceTwo)
				case "gone":
					w.WriteHeader(http.StatusGone)
				default:
					writeHandshake(w, instanceOne)
					if test.scenario == "oversized" {
						_, _ = fmt.Fprintf(w, "id: 9\nevent: activity\ndata: %s\n\n", strings.Repeat("x", 1<<20))
					} else {
						_, _ = fmt.Fprint(w, "id: 9\nevent: activity\ndata: {bad}\n\n")
					}
				}
			}))
			defer server.Close()
			conn, err := testConsole(t, server).connect(Pod{})
			if err != nil {
				t.Fatal(err)
			}
			admitted := false
			if err := conn.stream(t.Context(), instanceOne, func() { admitted = true }); err == nil ||
				admitted != test.admitted {
				t.Fatalf("invalid activity: admitted=%v error=%v", admitted, err)
			}
		})
	}
}

type testConsoleFactory struct {
	client *http.Client
	base   *url.URL
}

func (f testConsoleFactory) ForPod(Pod) (*http.Client, *url.URL, error) {
	return f.client, f.base, nil
}

func testConsole(t *testing.T, server *httptest.Server) *Console {
	t.Helper()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	console, err := NewConsole(testConsoleFactory{server.Client(), base}, testConsoleKey)
	if err != nil {
		t.Fatal(err)
	}
	console.timeout = 5 * time.Second
	return console
}

type instanceBody struct {
	InstanceID string                 `json:"instanceId"`
	Policy     string                 `json:"tracePersistencePolicy"`
	Namespaces []NamespaceObservation `json:"namespaces"`
}

func writeSnapshot(w http.ResponseWriter, instance, defaultPolicy string, rows []NamespaceObservation) {
	w.Header().Set(instanceHeader, instance)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(instanceBody{instance, defaultPolicy, rows})
}

func writeBaseline(w http.ResponseWriter, instance string) {
	w.Header().Set(instanceHeader, instance)
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"items":[],"hasMore":false,"resumeCursor":"8"}`)
}

func writeHandshake(w http.ResponseWriter, instance string) {
	w.Header().Set(instanceHeader, instance)
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(w, "event: handshake\ndata: {\"instanceId\":%q,\"afterCursor\":\"8\"}\n\n", instance)
	w.(http.Flusher).Flush()
}

func requireActivityRequest(t *testing.T, r *http.Request, instance string) {
	t.Helper()
	if r.Header.Get("Accept") != "text/event-stream" || r.URL.Query().Get("afterCursor") != "8" ||
		r.URL.Query().Get("instanceId") != instance || r.Header.Get("Last-Event-ID") != "" {
		t.Error("invalid Console activity request")
	}
}

func waitContext(ctx context.Context, signal <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return false
	case <-signal:
		return true
	}
}
