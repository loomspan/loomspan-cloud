package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPodWatchDerivesCurrentEligibilityFields(t *testing.T) {
	resource := podResource{}
	resource.Metadata.Namespace = "loomspan-engine"
	resource.Metadata.Name = "pod-a"
	resource.Metadata.UID = "uid-a"
	resource.Status.PodIP = "10.42.0.1"
	resource.Status.Conditions = append(resource.Status.Conditions, podCondition{Type: "Ready", Status: "True"})
	pod := selectPod(map[string]podResource{"uid-a": resource})
	if pod == nil || pod.Name != "pod-a" || pod.UID != "uid-a" || !pod.Ready || pod.Terminating {
		t.Fatalf("unexpected pod: %+v", pod)
	}
	resource.Metadata.DeletionTimestamp = new("2026-11-24T01:00:00Z")
	pod = selectPod(map[string]podResource{"uid-a": resource})
	if pod == nil || !pod.Terminating {
		t.Fatalf("terminating pod was not reported: %+v", pod)
	}
	if pod := selectPod(map[string]podResource{"uid-a": resource, "uid-b": resource}); pod != nil {
		t.Fatalf("ambiguous pod selection: %+v", pod)
	}
}

func TestPodWatchRelistsOnExpiredResourceVersion(t *testing.T) {
	for _, end := range []string{"http-410", "event-error", "malformed", "eof"} {
		t.Run(end, func(t *testing.T) {
			token := filepath.Join(t.TempDir(), "token")
			writeTestFile(t, token, []byte("token-one"))
			var lists, watches atomic.Int32
			loseWatch := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/namespaces/loomspan-engine/pods" ||
					r.URL.Query().Get("labelSelector") != EngineSelector {
					t.Error("watch did not scope its pod request")
				}
				if r.URL.Query().Get("watch") != "true" {
					list := lists.Add(1)
					expected := "Bearer token-one"
					uid, version := "old", "10"
					if list > 1 {
						expected, uid, version = "Bearer token-two", "new", "20"
					}
					if r.Header.Get("Authorization") != expected {
						t.Error("pod list used a stale service token")
					}
					_, _ = fmt.Fprintf(w, `{"metadata":{"resourceVersion":%q},"items":[%s]}`,
						version, testPodJSON(uid, version, false, false))
					return
				}
				number := watches.Add(1)
				query := r.URL.Query()
				version := "10"
				if number > 1 {
					version = "20"
				}
				if query.Get("resourceVersion") != version || query.Get("allowWatchBookmarks") != "true" {
					t.Error("watch did not use the list resource version and request bookmarks")
				}
				if number == 1 && end == "http-410" {
					writeTestFile(t, token, []byte("token-two"))
					w.WriteHeader(http.StatusGone)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				if number == 1 {
					select {
					case <-r.Context().Done():
						return
					case <-loseWatch:
					}
					switch end {
					case "event-error":
						_, _ = fmt.Fprintln(w, `{"type":"ERROR","object":{"code":410}}`)
					case "malformed":
						_, _ = fmt.Fprintln(w, `{"type":`)
					}
					return
				}
				_, _ = fmt.Fprintln(w, `{"type":"BOOKMARK","object":{"metadata":{"resourceVersion":"21"}}}`)
				_, _ = fmt.Fprintf(w, "{\"type\":\"MODIFIED\",\"object\":%s}\n", testPodJSON("new", "22", true, false))
				_, _ = fmt.Fprintf(w, "{\"type\":\"MODIFIED\",\"object\":%s}\n", testPodJSON("new", "23", false, true))
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			api, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			updates := make(chan *Pod)
			watch := &PodWatch{server.Client(), api, token, EngineNamespace, EngineSelector, time.Millisecond}
			go watch.Run(ctx, updates, nil)
			next := func() *Pod {
				t.Helper()
				select {
				case pod := <-updates:
					return pod
				case <-ctx.Done():
					t.Fatal("watch update timed out")
					return nil
				}
			}
			if end != "http-410" {
				if pod := next(); pod == nil || pod.UID != "old" || pod.Ready {
					t.Fatalf("unexpected first candidate: %+v", pod)
				}
				writeTestFile(t, token, []byte("token-two"))
				close(loseWatch)
				if pod := next(); pod != nil {
					t.Fatalf("watch loss did not remove the candidate: %+v", pod)
				}
			}
			if pod := next(); pod == nil || pod.UID != "new" || pod.Ready {
				t.Fatalf("replacement pod not observed: %+v", pod)
			}
			if pod := next(); pod == nil || pod.UID != "new" || !pod.Ready {
				t.Fatalf("readiness update not observed: %+v", pod)
			}
			if pod := next(); pod == nil || !pod.Terminating || pod.Ready {
				t.Fatalf("termination update not observed: %+v", pod)
			}
		})
	}
}

func testPodJSON(uid, version string, ready, terminating bool) string {
	readiness := "False"
	if ready {
		readiness = "True"
	}
	deletion := "null"
	if terminating {
		deletion = `"2026-11-24T01:00:00Z"`
	}
	return fmt.Sprintf(`{"metadata":{"name":"same","namespace":"loomspan-engine","uid":%q,"resourceVersion":%q,"deletionTimestamp":%s},"status":{"podIP":"10.42.0.1","conditions":[{"type":"Ready","status":%q}]}}`,
		uid, version, deletion, readiness)
}
