package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
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
)

func TestTransport(t *testing.T) {
	issuer := newTestIssuer(t)
	wrongIssuer := newTestIssuer(t)
	client := issuer.leaf(t, 2, "", x509.ExtKeyUsageClientAuth)
	pemFiles := writeClientFiles(t, issuer, client)

	paths := []string{"/internal/v1/validate", consolePath + "/instance"}
	for _, test := range []struct {
		name, dns string
		ca        testIssuer
		wantError bool
	}{
		{"verified-engine", EngineServerName, issuer, false},
		{"trusted-wrong-dns", "different.loomspan-engine.svc", issuer, true},
		{"untrusted-engine", EngineServerName, wrongIssuer, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int64
			server := tlsTestServer(t, test.ca.leaf(t, 2, test.dns, x509.ExtKeyUsageServerAuth),
				issuer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Header.Get(consoleKey) != "" {
						t.Error("transport supplied a Console key")
					}
					w.WriteHeader(http.StatusNoContent)
				}))
			httpClient, base := testPodClient(t, server, pemFiles)
			for _, route := range paths {
				response, err := httpClient.Get(base.JoinPath(route).String())
				if test.wantError {
					if err == nil {
						response.Body.Close()
						t.Fatalf("%s accepted the wrong server identity", route)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != http.StatusNoContent {
					t.Fatalf("unexpected response code: %d", response.StatusCode)
				}
			}
			want := int64(len(paths))
			if test.wantError {
				want = 0
			}
			if got := requests.Load(); got != want {
				t.Fatalf("received %d requests, want %d", got, want)
			}
		})
	}
}

func TestTransportRejectsRedirectHostsAndReloadsClientCredentials(t *testing.T) {
	issuer := newTestIssuer(t)
	files := writeClientFiles(t, issuer, issuer.leaf(t, 7, "", x509.ExtKeyUsageClientAuth))
	redirects := atomic.Int64{}
	server := tlsTestServer(t, issuer.leaf(t, 2, EngineServerName, x509.ExtKeyUsageServerAuth),
		issuer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/redirect" {
				http.Redirect(w, r, "https://127.0.0.1:1/other-target", http.StatusTemporaryRedirect)
				return
			}
			redirects.Add(1)
			_, _ = io.WriteString(w, r.TLS.PeerCertificates[0].SerialNumber.String())
		}))
	client, base := testPodClient(t, server, files)
	if response, err := client.Get(base.JoinPath("redirect").String()); err != nil {
		t.Fatal(err)
	} else {
		response.Body.Close()
		if response.StatusCode != http.StatusTemporaryRedirect {
			t.Fatalf("redirect was followed: %d", response.StatusCode)
		}
	}

	for _, rawURL := range []string{"http://" + base.Host + "/x", "https://127.0.0.1:1/x"} {
		if response, err := client.Get(rawURL); err == nil {
			response.Body.Close()
			t.Fatalf("transport accepted an unbound address %s", rawURL)
		}
	}
	request, err := http.NewRequest(http.MethodGet, base.JoinPath("/x").String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "unrelated.example"
	if response, err := client.Do(request); err == nil {
		response.Body.Close()
		t.Fatal("transport accepted an alternate Host header")
	}

	assertSerial := func(expected string) {
		t.Helper()
		response, err := client.Get(base.JoinPath("serial").String())
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || string(body) != expected {
			t.Fatalf("received serial %q, want %q: %v", body, expected, err)
		}
	}
	assertSerial("7")
	client.CloseIdleConnections()
	renewed := issuer.leaf(t, 8, "", x509.ExtKeyUsageClientAuth)
	writeTestFile(t, files.cert, renewed.cert)
	writeTestFile(t, files.key, renewed.key)
	assertSerial("8")
	client.CloseIdleConnections()
	writeTestFile(t, files.key, []byte("incomplete test key"))
	if response, err := client.Get(base.JoinPath("serial").String()); err == nil {
		response.Body.Close()
		t.Fatal("transport silently reused the replaced client key")
	}
	if redirects.Load() != 2 {
		t.Fatalf("unexpected number of authenticated receiver calls: %d", redirects.Load())
	}
}

func TestTransportConstructsIPv6PodURL(t *testing.T) {
	issuer := newTestIssuer(t)
	files := writeClientFiles(t, issuer, issuer.leaf(t, 2, "", x509.ExtKeyUsageClientAuth))
	transport, err := NewTransport(EngineServerName, 8443, files.ca, files.cert, files.key)
	if err != nil {
		t.Fatal(err)
	}
	_, base, err := transport.ForPod(Pod{Address: "fd00::a"})
	if err != nil || base.String() != "https://[fd00::a]:8443" {
		t.Fatalf("unexpected IPv6 pod URL %v: %v", base, err)
	}
}

type testIssuer struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

type testLeaf struct {
	cert []byte
	key  []byte
}

func newTestIssuer(t *testing.T) testIssuer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "transport test issuer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testIssuer{cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (issuer testIssuer) leaf(t *testing.T, serial int64, dns string, usage x509.ExtKeyUsage) testLeaf {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		NotBefore:    issuer.cert.NotBefore,
		NotAfter:     issuer.cert.NotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	if dns != "" {
		template.DNSNames = []string{dns}
	}
	cert, err := x509.CreateCertificate(rand.Reader, template, issuer.cert, &key.PublicKey, issuer.key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return testLeaf{
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}),
	}
}

type clientFiles struct {
	ca   string
	cert string
	key  string
}

func writeClientFiles(t *testing.T, ca testIssuer, client testLeaf) clientFiles {
	t.Helper()
	directory := t.TempDir()
	files := clientFiles{filepath.Join(directory, "ca.crt"), filepath.Join(directory, "tls.crt"), filepath.Join(directory, "tls.key")}
	writeTestFile(t, files.ca, ca.pem)
	writeTestFile(t, files.cert, client.cert)
	writeTestFile(t, files.key, client.key)
	return files
}

func writeTestFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func tlsTestServer(t *testing.T, leaf testLeaf, ca testIssuer, handler http.Handler) *httptest.Server {
	t.Helper()
	cert, err := tls.X509KeyPair(leaf.cert, leaf.key)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    roots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func testPodClient(t *testing.T, server *httptest.Server, files clientFiles) (*http.Client, *url.URL) {
	t.Helper()
	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewTransport(EngineServerName, uint16(number), files.ca, files.cert, files.key)
	if err != nil {
		t.Fatal(err)
	}
	client, base, err := transport.ForPod(Pod{Address: host})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client, base
}
