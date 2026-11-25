//go:build k3s_integration

package invocation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
	"github.com/loomspan/loomspan-cloud/management-backend/internal/publication"
)

const instancePath = "/_loomspan/observability/v1/instance"
const publishPath = "/internal/v1/publish"
const invokePath = "/internal/v1/invoke"

// milestoneTransport places barriers on independently authenticated TLS traffic.
// Only test-controlled display of snapshots is delayed, never the Java installation.
type milestoneTransport struct {
	base engine.PodClientFactory
	mu   sync.Mutex

	gates           map[string]*snapshotBarrier
	replacementFrom string
	restoring       string
	acknowledgments []milestoneAck
	calls           []milestoneCall
}

type milestoneAck struct {
	uid      string
	metadata milestoneMetadata
}

type milestoneMetadata struct {
	Namespace          string                      `json:"namespace"`
	PublicationVersion string                      `json:"publicationVersion"`
	RuntimeSettings    publication.RuntimeSettings `json:"runtimeSettings"`
	VariableValues     map[string]string           `json:"variableValues"`
}

type milestoneCall struct {
	uid      string
	request  request
	response map[string]json.RawMessage
}

type snapshotBarrier struct {
	attempted chan struct{}
	released  chan struct{}
	attempt   sync.Once
	unlock    sync.Once
}

func (g *snapshotBarrier) release() {
	g.unlock.Do(func() { close(g.released) })
}

func newMilestoneTransport(base engine.PodClientFactory) *milestoneTransport {
	return &milestoneTransport{base: base, gates: make(map[string]*snapshotBarrier)}
}

func (m *milestoneTransport) ForPod(pod engine.Pod) (*http.Client, *url.URL, error) {
	client, address, err := m.base.ForPod(pod)
	if err != nil {
		return nil, nil, err
	}
	wrapper := *client
	wrapper.Transport = &milestoneRoundTrip{test: m, base: client.Transport, uid: pod.UID}
	return &wrapper, address, nil
}

func (m *milestoneTransport) holdSnapshots(uid string) *snapshotBarrier {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addGate(uid)
}

func (m *milestoneTransport) addGate(uid string) *snapshotBarrier {
	if current := m.gates[uid]; current != nil {
		return current
	}
	gate := &snapshotBarrier{attempted: make(chan struct{}), released: make(chan struct{})}
	m.gates[uid] = gate
	return gate
}

func (m *milestoneTransport) holdReplacementOnPublish(old, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replacementFrom, m.restoring = old, version
}

func (m *milestoneTransport) snapshotGate(uid string) *snapshotBarrier {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gates[uid]
}

func (m *milestoneTransport) releaseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, gate := range m.gates {
		gate.release()
	}
}

func (m *milestoneTransport) requireAcknowledgment(t *testing.T, uid, version string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	expected, err := publication.EffectiveRuntimeSettings("NEVER")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, ack := range m.acknowledgments {
		if ack.uid != uid || ack.metadata.PublicationVersion != version {
			continue
		}
		seen++
		if ack.metadata.Namespace != milestoneApp || ack.metadata.VariableValues == nil || len(ack.metadata.VariableValues) != 0 ||
			!reflect.DeepEqual(ack.metadata.RuntimeSettings, expected) {
			t.Fatal("Java acknowledgment did not use complete milestone runtime settings and an explicit empty variable map")
		}
	}
	if seen != 1 {
		t.Fatalf("authenticated Java acknowledgment count for pod %s and version %s = %d", uid, version, seen)
	}
}

func (m *milestoneTransport) invocations() []milestoneCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]milestoneCall(nil), m.calls...)
}

type milestoneRoundTrip struct {
	test *milestoneTransport
	base http.RoundTripper
	uid  string
}

func (r *milestoneRoundTrip) CloseIdleConnections() {
	if closer, ok := r.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (r *milestoneRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case publishPath:
		return r.publish(req)
	case invokePath:
		return r.invoke(req)
	case instancePath:
		return r.snapshot(req)
	default:
		return r.base.RoundTrip(req)
	}
}

func (r *milestoneRoundTrip) publish(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	data, err := replayFixtureBody(&req.Body)
	if err != nil {
		return nil, err
	}
	meta, err := parseFixtureMetadata(req.Header.Get("Content-Type"), data)
	if err != nil {
		return nil, err
	}
	r.test.mu.Lock()
	if r.test.restoring != "" && r.uid != r.test.replacementFrom && meta.PublicationVersion == r.test.restoring {
		r.test.addGate(r.uid)
	}
	r.test.mu.Unlock()
	response, err := r.base.RoundTrip(req)
	if err == nil && response.StatusCode == http.StatusNoContent &&
		response.TLS != nil && len(response.TLS.VerifiedChains) > 0 {
		r.test.mu.Lock()
		r.test.acknowledgments = append(r.test.acknowledgments, milestoneAck{r.uid, meta})
		r.test.mu.Unlock()
	}
	return response, err
}

func (r *milestoneRoundTrip) invoke(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	data, err := replayFixtureBody(&req.Body)
	if err != nil {
		return nil, err
	}
	var outbound request
	if err := json.Unmarshal(data, &outbound); err != nil {
		return nil, err
	}
	r.test.mu.Lock()
	index := len(r.test.calls)
	r.test.calls = append(r.test.calls, milestoneCall{uid: r.uid, request: outbound})
	r.test.mu.Unlock()
	response, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := replayFixtureBody(&response.Body)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	r.test.mu.Lock()
	r.test.calls[index].response = fields
	r.test.mu.Unlock()
	return response, nil
}

func (r *milestoneRoundTrip) snapshot(req *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	// Buffer the real snapshot before checking the gate, including if the
	// response headers were received before the gate was installed.
	if _, err := replayFixtureBody(&response.Body); err != nil {
		return nil, err
	}
	if gate := r.test.snapshotGate(r.uid); gate != nil {
		gate.attempt.Do(func() { close(gate.attempted) })
		select {
		case <-gate.released:
		case <-req.Context().Done():
			response.Body.Close()
			return nil, req.Context().Err()
		}
	}
	return response, nil
}

func replayFixtureBody(body *io.ReadCloser) ([]byte, error) {
	data, err := io.ReadAll(*body)
	(*body).Close()
	if err == nil {
		*body = io.NopCloser(bytes.NewReader(data))
	}
	return data, err
}

func parseFixtureMetadata(contentType string, data []byte) (milestoneMetadata, error) {
	media, parameters, err := mime.ParseMediaType(contentType)
	if err != nil || media != "multipart/form-data" || parameters["boundary"] == "" {
		return milestoneMetadata{}, errors.New("publication probe expected multipart metadata")
	}
	parts := multipart.NewReader(bytes.NewReader(data), parameters["boundary"])
	for {
		part, err := parts.NextPart()
		if err != nil {
			return milestoneMetadata{}, err
		}
		if part.FormName() == "metadata" {
			var meta milestoneMetadata
			err := json.NewDecoder(part).Decode(&meta)
			return meta, err
		}
		part.Close()
	}
}
