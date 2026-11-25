package invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
	"github.com/loomspan/loomspan-cloud/management-backend/internal/publication"
)

type grantsFunc func(context.Context, string, Actor) (Grant, error)

func (f grantsFunc) Lookup(ctx context.Context, applicationID string, actor Actor) (Grant, error) {
	return f(ctx, applicationID, actor)
}

type placementsFunc func(context.Context, string) (publication.Placement, error)

func (f placementsFunc) Eligible(ctx context.Context, applicationID string) (publication.Placement, error) {
	return f(ctx, applicationID)
}

type factoryFunc func(engine.Pod) (*http.Client, *url.URL, error)

func (f factoryFunc) ForPod(pod engine.Pod) (*http.Client, *url.URL, error) {
	return f(pod)
}

type tryRoundTrip func(*http.Request) (*http.Response, error)

func (f tryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

var fixtureActor = Actor{Kind: DirectUser, ID: "user:id"}
var fixturePlacement = publication.Placement{
	Pod:       engine.Pod{Namespace: "engine", Name: "pod", UID: "pod-1", Address: "127.0.0.1", Ready: true},
	Namespace: "fixture", Version: "version-one", GenerationID: "generation-one", InstanceID: "jvm-one", ConnectionID: 1,
}

func allowFixture(context.Context, string, Actor) (Grant, error) {
	return Grant{Namespace: fixturePlacement.Namespace}, nil
}

func placeFixture(context.Context, string) (publication.Placement, error) {
	return fixturePlacement, nil
}

func fixtureService(t *testing.T, grants grantsFunc, place placementsFunc, transport factoryFunc) *Service {
	t.Helper()
	service, err := NewService(grants, place, transport, "verified-cloud")
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func serverFactory(t *testing.T, handle http.HandlerFunc) factoryFunc {
	t.Helper()
	server := httptest.NewTLSServer(handle)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return func(engine.Pod) (*http.Client, *url.URL, error) { return server.Client(), base, nil }
}

func TestInvokeAuthorization(t *testing.T) {
	var dispatches atomic.Int32
	wire := make(chan request, 5)
	transport := serverFactory(t, func(w http.ResponseWriter, req *http.Request) {
		dispatches.Add(1)
		var outbound request
		if req.URL.Path != "/internal/v1/invoke" || req.Method != "POST" || json.NewDecoder(req.Body).Decode(&outbound) != nil {
			http.Error(w, "invalid fixture request", 400)
			return
		}
		wire <- outbound
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"result":"{\"inner\":true}"}`)
	})
	backend := Actor{Kind: CallingBackend, ID: "authorized-backend"}
	grants := func(_ context.Context, app string, actor Actor) (Grant, error) {
		switch {
		case app == "app-one" && actor == fixtureActor:
			return Grant{Namespace: "fixture", Roles: []string{"reviewer"}}, nil
		case app == "app-two" && actor == fixtureActor:
			return Grant{Namespace: "fixture", Roles: []string{}}, nil
		case app == "app-one" && actor == backend:
			return Grant{Namespace: "fixture"}, nil
		default:
			return Grant{}, ErrAccessDenied
		}
	}
	svc := fixtureService(t, grants, placeFixture, transport)
	body := `{"input":{"number":9007199254740993},"executionView":"always","issuer":"fake","roles":["owner"],"endUser":{"id":"fake","roles":["owner"]}}`
	result, err := svc.InvokeJSON(t.Context(), fixtureActor, "app-one", "exact-local-name", strings.NewReader(body))
	if err != nil || result.Body.Result == nil || *result.Body.Result != `{"inner":true}` {
		t.Fatalf("direct response = %+v, %v", result, err)
	}
	first := <-wire
	if first.Namespace != "fixture" || first.Skill != "exact-local-name" || first.Caller.Issuer != "verified-cloud" ||
		first.Caller.Subject != fixtureActor.ID || len(first.Caller.Roles) != 1 || first.Caller.Roles[0] != "reviewer" ||
		first.ExecutionView != "" || string(first.Input) != `{"number":9007199254740993}` {
		t.Fatalf("direct wire identity or payload changed: %+v", first)
	}
	if _, err := svc.InvokeJSON(t.Context(), fixtureActor, "app-two", "skill", strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if second := <-wire; second.Caller.Roles == nil || len(second.Caller.Roles) != 0 {
		t.Fatalf("explicit empty roles = %#v", second.Caller.Roles)
	}
	claim := `{"input":{},"issuer":"forged","endUser":{"id":"outside/user\ud83d\ude00","roles":["member\\ud800"]}}`
	if _, err := svc.InvokeJSON(t.Context(), backend, "app-one", "skill", strings.NewReader(claim)); err != nil {
		t.Fatal(err)
	}
	if third := <-wire; third.Caller.Issuer != backend.ID || third.Caller.Subject != "outside/user😀" ||
		len(third.Caller.Roles) != 1 || third.Caller.Roles[0] != `member\ud800` {
		t.Fatalf("backend wire identity = %+v", third.Caller)
	}
	for _, denied := range []struct {
		actor Actor
		app   string
	}{
		{backend, "app-two"},
		{Actor{Kind: CallingBackend, ID: "verified-cloud"}, "app-one"},
		{Actor{Kind: DirectUser, ID: "unknown"}, "app-one"},
		{Actor{Kind: DirectUser, ID: " "}, "app-one"},
	} {
		if _, err := svc.InvokeJSON(t.Context(), denied.actor, denied.app, "skill", strings.NewReader(claim)); err != ErrAccessDenied {
			t.Fatalf("denied actor = %v", err)
		}
	}
	for _, malformed := range []string{`{"input":{}}`, `{"input":{},"endUser":{"id":" ","roles":[]}}`,
		`{"input":{},"endUser":{"id":"outside","roles":null}}`, `{"input":{},"endUser":{"id":"outside","roles":[" "]}}`,
		"{\"input\":{},\"endUser\":{\"id\":\"outside\xff\",\"roles\":[]}}",
		"{\"input\":{},\"endUser\":{\"id\":\"outside\",\"roles\":[\"member\xff\"]}}",
		`{"input":{},"endUser":{"id":"outside\ud800","roles":[]}}`,
		`{"input":{},"endUser":{"id":"outside","roles":["member\udc00"]}}`} {
		if _, err := svc.InvokeJSON(t.Context(), backend, "app-one", "skill", strings.NewReader(malformed)); err != ErrInvalidRequest {
			t.Fatalf("malformed backend assertion = %v", err)
		}
	}
	if count := dispatches.Load(); count != 3 {
		t.Fatalf("dispatches = %d, want 3", count)
	}
}

type repeatingSpaces struct{ read int }

func (r *repeatingSpaces) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	r.read += len(p)
	return len(p), nil
}

func TestInvokeJSONBodyLimit(t *testing.T) {
	var dispatches atomic.Int32
	transport := serverFactory(t, func(w http.ResponseWriter, req *http.Request) {
		dispatches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"result":"ok"}`)
	})
	svc := fixtureService(t, allowFixture, placeFixture, transport)
	valid := `{"input":{},"unused":"metadata counts"}`
	max := &repeatingSpaces{}
	body := io.MultiReader(strings.NewReader(valid), io.LimitReader(max, MaxBodyBytes-int64(len(valid))))
	if _, err := svc.InvokeJSON(t.Context(), fixtureActor, "app", "skill", body); err != nil {
		t.Fatalf("exact byte limit = %v", err)
	}
	over := &repeatingSpaces{}
	body = io.MultiReader(strings.NewReader(valid), over)
	if _, err := svc.InvokeJSON(t.Context(), fixtureActor, "app", "skill", body); err != ErrBodyTooLarge {
		t.Fatalf("oversized body = %v", err)
	}
	if over.read+len(valid) != MaxBodyBytes+1 || MaxBodyBytes != 20971520 || MaxUploadedFiles != 25 {
		t.Fatalf("limits were not applied to complete body; consumed %d", over.read+len(valid))
	}
	for _, invalid := range []string{`{"input":{}} {}`, `{"input":null}`, `{"input":[{}]}`, `{}`} {
		if _, err := svc.InvokeJSON(t.Context(), fixtureActor, "app", "skill", strings.NewReader(invalid)); err != ErrInvalidRequest {
			t.Fatalf("invalid payload %s = %v", invalid, err)
		}
	}
	if dispatches.Load() != 1 {
		t.Fatalf("transport ran %d times", dispatches.Load())
	}
}

func TestInvokeDiagnosticProjection(t *testing.T) {
	const executionView = `{"sessionId":"fixture","events":[{"timestamp":"2026-01-01T01:00:00Z","details":{"nested":[9007199254740993,null]}}]}`
	const diagnostics = `{"message":"the original message","debug":"root\n\tCaused by: original","diagnosis":{"fields":{"nested":["a",1]}},"issues":[{"code":"REQUIRED","path":"/text"}],"executionView":` + executionView + `}`
	var mode atomic.Bool
	send := make(chan request, 5)
	transport := serverFactory(t, func(w http.ResponseWriter, req *http.Request) {
		var outbound request
		if err := json.NewDecoder(req.Body).Decode(&outbound); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		send <- outbound
		w.Header().Set("Content-Type", "application/json")
		switch string(outbound.Input) {
		case `{"text":"fail"}`:
			w.WriteHeader(422)
			io.WriteString(w, diagnostics)
		case `{}`:
			w.WriteHeader(400)
			io.WriteString(w, `{"message":"missing input","issues":[{"path":"/text"}]}`)
		default:
			io.WriteString(w, `{"result":"the exact result","executionView":`+executionView+`,"debug":"never on success"}`)
		}
	})
	grants := func(context.Context, string, Actor) (Grant, error) {
		return Grant{Namespace: "fixture", ShowExecution: mode.Load()}, nil
	}
	svc := fixtureService(t, grants, placeFixture, transport)
	for _, show := range []bool{false, true} {
		mode.Store(show)
		for _, success := range []bool{true, false} {
			input, expectedStatus := `{"input":{"text":"fail"},"executionView":"always"}`, 422
			if success {
				input, expectedStatus = `{"input":{"text":"ping"}}`, 200
			}
			result, err := svc.InvokeJSON(t.Context(), fixtureActor, "app", "skill", strings.NewReader(input))
			if err != nil || result.StatusCode != expectedStatus {
				t.Fatalf("show=%v, success=%v: %+v, %v", show, success, result, err)
			}
			outbound := <-send
			if (outbound.ExecutionView == "always") != show {
				t.Fatalf("wire view mode = %q (show=%v)", outbound.ExecutionView, show)
			}
			got := result.Body
			if show && string(got.ExecutionView) != executionView || !show && got.ExecutionView != nil {
				t.Fatalf("show=%v, view = %s", show, got.ExecutionView)
			}
			if success {
				if got.Result == nil || *got.Result != "the exact result" || got.Debug != nil || got.Diagnosis != nil {
					t.Fatalf("success projection = %+v", got)
				}
			} else if string(got.Message) != `"the original message"` || !bytes.Contains(got.Issues, []byte("REQUIRED")) ||
				(show != (got.Debug != nil && bytes.Contains(got.Diagnosis, []byte("nested")))) {
				t.Fatalf("failure projection = %+v", got)
			}
		}
	}
	fallback, err := svc.InvokeJSON(t.Context(), fixtureActor, "app", "skill", strings.NewReader(`{"input":{}}`))
	if err != nil || fallback.StatusCode != 400 || string(fallback.Body.ExecutionView) != "null" ||
		!bytes.Contains(fallback.Body.Issues, []byte("text")) || string(fallback.Body.Message) != `"missing input"` {
		t.Fatalf("null view changed failure = %+v, %v", fallback, err)
	}
}

func TestInvokeOutcome(t *testing.T) {
	base := &url.URL{Scheme: "https", Host: "192.0.2.1"}
	t.Run("unavailable or superseded before dispatch", func(t *testing.T) {
		calls := 0
		factory := func(engine.Pod) (*http.Client, *url.URL, error) {
			return &http.Client{Transport: tryRoundTrip(func(*http.Request) (*http.Response, error) {
				t.Error("unavailable placement dispatched")
				return nil, io.EOF
			})}, base, nil
		}
		for _, failOn := range []int{1, 2} {
			calls = 0
			lookup := func(context.Context, string) (publication.Placement, error) {
				calls++
				if calls == failOn {
					return publication.Placement{}, publication.ErrPlacementUnavailable
				}
				return fixturePlacement, nil
			}
			svc := fixtureService(t, allowFixture, lookup, factory)
			if _, err := svc.InvokeJSON(t.Context(), fixtureActor, "app", "skill", strings.NewReader(`{"input":{}}`)); err != ErrUnavailable {
				t.Fatalf("unavailable at lookup %d = %v", failOn, err)
			}
		}
	})
	t.Run("response from verified TLS connection", func(t *testing.T) {
		var requests atomic.Int32
		transport := serverFactory(t, func(w http.ResponseWriter, req *http.Request) {
			requests.Add(1)
			switch requests.Load() {
			case 1:
				w.WriteHeader(503)
			case 2:
				http.Redirect(w, req, "/redirected", 307)
			case 3:
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"result":`)
			case 4:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(500)
				io.WriteString(w, `{"message":"provider responded 503"}`)
			}
		})
		svc := fixtureService(t, allowFixture, placeFixture, transport)
		for index := range 4 {
			result, err := svc.InvokeJSON(t.Context(), fixtureActor, "app", "skill", strings.NewReader(`{"input":{}}`))
			switch index {
			case 0:
				if result.StatusCode != 503 || !result.SafeToRetry || err != nil {
					t.Fatalf("verified empty 503 = %+v, %v", result, err)
				}
			case 1, 2:
				if err != ErrOutcomeUnknown {
					t.Fatalf("unconfirmed redirect/truncation = %v", err)
				}
			case 3:
				if result.StatusCode != 500 || result.SafeToRetry || err != nil {
					t.Fatalf("execution failure = %+v, %v", result, err)
				}
			}
		}
		if requests.Load() != 4 {
			t.Fatalf("response handling initiated replay, requests = %d", requests.Load())
		}
	})
	for _, kind := range []string{"unverified 503", "EOF", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			transport := func(engine.Pod) (*http.Client, *url.URL, error) {
				return &http.Client{Transport: tryRoundTrip(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.GetBody != nil {
						t.Error("request enabled replay")
					}
					switch kind {
					case "unverified 503":
						return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}},
							Body: io.NopCloser(strings.NewReader(`{"message":"looks like the engine"}`))}, nil
					case "EOF":
						return nil, io.EOF
					default:
						cancel()
						return nil, req.Context().Err()
					}
				})}, base, nil
			}
			svc := fixtureService(t, allowFixture, placeFixture, transport)
			_, err := svc.InvokeJSON(ctx, fixtureActor, "app", "skill", strings.NewReader(`{"input":{}}`))
			if !errors.Is(err, ErrOutcomeUnknown) || calls != 1 {
				t.Fatalf("expected one dispatch with unknown outcome, got %d, %v", calls, err)
			}
		})
	}
}
