package publication

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

type podTransportFunc func(engine.Pod) (*http.Client, *url.URL, error)

func (f podTransportFunc) ForPod(p engine.Pod) (*http.Client, *url.URL, error) {
	return f(p)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testMTLSTransport(t *testing.T, handler http.HandlerFunc) *engine.Transport {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	clients := x509.NewCertPool()
	if !clients.AppendCertsFromPEM(certPEM) {
		t.Fatal("test client certificate is invalid")
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clients, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	addr := server.Listener.Addr().(*net.TCPAddr)
	dir := t.TempDir()
	write := func(name string, contents []byte) string {
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	ca := write("server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	certFile := write("client.pem", certPEM)
	keyFile := write("client-key.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if len(server.Certificate().DNSNames) == 0 {
		t.Fatal("test server certificate has no DNS identity")
	}
	transport, err := engine.NewTransport(server.Certificate().DNSNames[0], uint16(addr.Port), ca, certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func TestEngineClient(t *testing.T) {
	original := emptyPack(t, "sales")
	settings, _ := EffectiveRuntimeSettings(TraceOnError)
	target := testProjection(1, false)
	observer := newFakeObserver(target)
	var call atomic.Int32
	transport := testMTLSTransport(t, func(w http.ResponseWriter, r *http.Request) {
		step := call.Add(1)
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || r.Method != http.MethodPost {
			t.Error("missing verified mTLS or invalid HTTP method")
		}
		got := readPublicationParts(t, r, original)
		if got.Namespace != "sales" || got.RuntimeSettings != settings {
			t.Error("wrong metadata on wire")
		}
		if r.URL.Path == validatePath {
			if got.PublicationVersion != "" || got.VariableValues != nil {
				t.Error("validate included publication-only fields")
			}
			switch step {
			case 1:
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"valid":true,"issues":[{"severity":"WARNING","sourceName":"loomspan/execution.yaml","message":"notice"}],"skills":[{"namespace":"sales","localName":"demo","kind":"EXECUTOR"}]}`)
			case 2:
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"valid":true,"issues":[],"skills":null}`)
			case 3:
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "validation interrupted")
			default:
				t.Errorf("unexpected validation request %d", step)
			}
			return
		}
		if r.URL.Path != publishPath || got.PublicationVersion != "opaque" || got.VariableValues == nil ||
			len(*got.VariableValues) != 0 || *got.VariableValues == nil {
			t.Error("publish omitted its reference-free map or requested version")
		}
		switch step {
		case 4:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "Unknown skill café\n\n")
		case 5:
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", strconv.Itoa(100))
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "partial")
		case 6:
			w.WriteHeader(http.StatusOK)
		case 7:
			ending := target
			ending.Terminating = true
			observer.set(ending)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected install request %d", step)
		}
	})
	client := NewEngineClient(observer, transport)
	ctx := context.Background()
	meta := engineMetadata{Namespace: "sales", RuntimeSettings: settings}
	report, failure := client.validate(ctx, target, original, meta)
	if failure != nil || !report.Valid || len(report.Issues) != 1 || len(report.Skills) != 1 {
		t.Fatalf("valid response became %+v, %v", report, failure)
	}
	for range 2 {
		report, failure := client.validate(ctx, target, original, meta)
		if failure == nil || len(report.Issues)+len(report.Skills) != 0 {
			t.Fatalf("invalid response became %+v, %v", report, failure)
		}
	}
	values := map[string]string{}
	meta.VariableValues, meta.PublicationVersion = &values, "opaque"
	if got := client.install(ctx, target, original, meta); got.status != InstallRejected || got.message != "Unknown skill café\n\n" {
		t.Fatalf("changed rejection: %+v", got)
	}
	for range 2 {
		if got := client.install(ctx, target, original, meta); got.status != InstallUnconfirmed {
			t.Fatalf("invalid acknowledgment was classified %+v", got)
		}
	}
	if got := client.install(ctx, target, original, meta); got.status != InstallAcknowledged {
		t.Fatalf("admitted reply was discarded after termination: %+v", got)
	}
	if got := client.install(ctx, target, original, meta); got.status != InstallNotSent || !got.shuttingDown {
		t.Fatalf("a later request crossed known termination: %+v", got)
	}
	if _, failure := client.validate(ctx, target, original, meta); failure == nil || !failure.ShuttingDown || call.Load() != 7 {
		t.Fatal("termination did not prevent later validation and publication")
	}
}

func readPublicationParts(t *testing.T, r *http.Request, original []byte) engineMetadata {
	t.Helper()
	var meta engineMetadata
	reader, err := r.MultipartReader()
	if err != nil {
		t.Error(err)
		return meta
	}
	pack, err := reader.NextPart()
	if err != nil {
		t.Error(err)
		return meta
	}
	data, err := io.ReadAll(pack)
	if err != nil || pack.FormName() != "pack" || pack.Header.Get("Content-Type") != "application/zip" || !bytes.Equal(data, original) {
		t.Error("original application/zip pack part was not preserved")
	}
	metadata, err := reader.NextPart()
	if err != nil {
		t.Error(err)
		return meta
	}
	if metadata.FormName() != "metadata" || metadata.Header.Get("Content-Type") != "application/json" {
		t.Error("wrong metadata part")
	}
	raw, err := io.ReadAll(metadata)
	if err != nil {
		t.Error(err)
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Error(err)
	}
	if r.URL.Path == validatePath && bytes.Contains(raw, []byte(`"variableValues"`)) {
		t.Error("validate sent a value map")
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Error("unexpected multipart appendage")
	}
	return meta
}

func TestEngineClientAdmissionAndAuthenticatedReceipt(t *testing.T) {
	ctx := context.Background()
	target := testProjection(1, true)
	observer := newFakeObserver(target)
	var dispatched int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		dispatched++
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: r}, nil
	})}
	base := &url.URL{Scheme: "https", Host: "engine"}
	transport := podTransportFunc(func(engine.Pod) (*http.Client, *url.URL, error) {
		return client, base, nil
	})
	wire := NewEngineClient(observer, transport)
	if got := wire.install(ctx, target, nil, engineMetadata{}); got.status != InstallUnconfirmed || dispatched != 1 {
		t.Fatalf("unauthenticated 204 was not treated as unconfirmed: %+v", got)
	}
	wire.transport = podTransportFunc(func(engine.Pod) (*http.Client, *url.URL, error) {
		transition := target
		transition.Terminating = true
		observer.set(transition)
		return client, base, nil
	})
	if got := wire.install(ctx, target, nil, engineMetadata{}); got.status != InstallNotSent || !got.shuttingDown || dispatched != 1 {
		t.Fatalf("termination entered before dispatch but was ignored: %+v", got)
	}
}
