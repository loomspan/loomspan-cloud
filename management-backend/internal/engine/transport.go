package engine

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const EngineServerName = "execution-engine.loomspan-engine.svc"
const transportMount = "/var/run/loomspan/engine-transport"

// Transport authenticates the engine service while dialing the exact address of a watched pod.
type Transport struct {
	serverName      string
	port            string
	roots           *x509.CertPool
	certificatePath string
	keyPath         string
}

func InClusterTransport() (*Transport, error) {
	port := uint64(8443)
	if value := os.Getenv("LOOMSPAN_ENGINE_PORT"); value != "" {
		var err error
		port, err = strconv.ParseUint(value, 10, 16)
		if err != nil {
			return nil, errors.New("LOOMSPAN_ENGINE_PORT must be a valid port")
		}
	}
	name := os.Getenv("LOOMSPAN_ENGINE_SERVER_NAME")
	if name == "" {
		name = EngineServerName
	}
	return NewTransport(name, uint16(port), filepath.Join(transportMount, "ca.crt"),
		filepath.Join(transportMount, "tls.crt"), filepath.Join(transportMount, "tls.key"))
}

func NewTransport(serverName string, port uint16, caPath, certificatePath, keyPath string) (*Transport, error) {
	if serverName == "" || net.ParseIP(serverName) != nil || port == 0 || certificatePath == "" || keyPath == "" {
		return nil, errors.New("engine transport requires a DNS identity, port, and client certificate paths")
	}
	roots, err := readRoots(caPath)
	if err != nil {
		return nil, err
	}
	transport := &Transport{serverName, strconv.Itoa(int(port)), roots, certificatePath, keyPath}
	if _, err := transport.clientCertificate(nil); err != nil {
		return nil, err
	}
	return transport, nil
}

func readRoots(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read trusted CA certificate")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("trusted CA file contains no valid certificates")
	}
	return roots, nil
}

// ForPod returns a client bound to this pod address and the HTTPS base URL for its API.
// The caller should close idle connections after its work with the client.
func (t *Transport) ForPod(pod Pod) (*http.Client, *url.URL, error) {
	address, err := netip.ParseAddr(pod.Address)
	if err != nil || address.Zone() != "" || address.IsUnspecified() {
		return nil, nil, errors.New("pod does not have a usable IP address")
	}
	host := net.JoinHostPort(address.String(), t.port)
	base := &url.URL{Scheme: "https", Host: host}
	network := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			ServerName:           t.serverName,
			RootCAs:              t.roots,
			MinVersion:           tls.VersionTLS12,
			GetClientCertificate: t.clientCertificate,
		},
		TLSHandshakeTimeout: 5 * time.Second,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &http.Client{
		Transport: &podRoundTripper{expectedHost: host, transport: network},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client, base, nil
}

func (t *Transport) clientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	certPath, keyPath := t.certificatePath, t.keyPath
	directory := filepath.Dir(certPath)
	if directory == filepath.Dir(keyPath) {
		// Read both files from the same projected Secret generation during rotation.
		generation, err := os.Readlink(filepath.Join(directory, "..data"))
		switch {
		case err == nil:
			if filepath.IsAbs(generation) || generation == "." || generation == ".." || strings.ContainsAny(generation, "/\\") {
				return nil, errors.New("invalid projected client certificate directory")
			}
			directory = filepath.Join(directory, generation)
			certPath = filepath.Join(directory, filepath.Base(certPath))
			keyPath = filepath.Join(directory, filepath.Base(keyPath))
		case !errors.Is(err, os.ErrNotExist):
			return nil, errors.New("cannot read projected client certificate directory")
		}
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, errors.New("cannot load engine client certificate and key")
	}
	return &cert, nil
}

type podRoundTripper struct {
	expectedHost string
	transport    *http.Transport
}

func (t *podRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || req.URL.Scheme != "https" || req.URL.Host != t.expectedHost ||
		req.URL.Opaque != "" || req.URL.User != nil ||
		(req.Host != "" && req.Host != t.expectedHost) {
		return nil, errors.New("engine request must use the selected pod HTTPS address")
	}
	return t.transport.RoundTrip(req)
}

func (t *podRoundTripper) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
}
