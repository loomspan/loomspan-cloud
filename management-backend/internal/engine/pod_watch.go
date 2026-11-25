package engine

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const (
	EngineNamespace = "loomspan-engine"
	EngineSelector  = "app.kubernetes.io/name=loomspan-execution-engine"
	serviceAccount  = "/var/run/secrets/kubernetes.io/serviceaccount"
)

type Pod struct {
	Namespace   string
	Name        string
	UID         string
	Address     string
	Ready       bool
	Terminating bool
}

func (p Pod) sameTarget(other Pod) bool {
	return p.Namespace == other.Namespace && p.Name == other.Name &&
		p.UID == other.UID && p.Address == other.Address
}

type PodWatch struct {
	client       *http.Client
	api          *url.URL
	tokenPath    string
	namespace    string
	selector     string
	retryTimeout time.Duration
}

func InClusterPodWatch() (*PodWatch, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("Kubernetes service address is unavailable")
	}
	roots, err := readRoots(path.Join(serviceAccount, "ca.crt"))
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}).DialContext,
			TLSClientConfig:       &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &PodWatch{
		client:       client,
		api:          &url.URL{Scheme: "https", Host: net.JoinHostPort(host, port)},
		tokenPath:    path.Join(serviceAccount, "token"),
		namespace:    EngineNamespace,
		selector:     EngineSelector,
		retryTimeout: time.Second,
	}, nil
}

// Run publishes the current single candidate, or nil when the authoritative watch is unavailable.
// It closes updates when it exits. The error callback receives diagnostics without credentials or bodies.
func (w *PodWatch) Run(ctx context.Context, updates chan<- *Pod, report func(error)) {
	defer close(updates)
	defer w.client.CloseIdleConnections()

	var previous *Pod
	send := func(pod *Pod) {
		if (previous == nil && pod == nil) || (previous != nil && pod != nil && *previous == *pod) {
			return
		}
		previous = pod
		select {
		case updates <- pod:
		case <-ctx.Done():
		}
	}
	for ctx.Err() == nil {
		err := w.listAndWatch(ctx, send)
		send(nil)
		if ctx.Err() != nil {
			break
		}
		if err != nil && report != nil {
			report(err)
		}
		if !retry(ctx, w.retryTimeout) {
			break
		}
	}
}

func (w *PodWatch) listAndWatch(ctx context.Context, send func(*Pod)) error {
	listContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := w.get(listContext, nil)
	if err != nil {
		return fmt.Errorf("engine pod list: %w", err)
	}
	var listed struct {
		Metadata struct {
			ResourceVersion string `json:"resourceVersion"`
			Continue        string `json:"continue"`
		} `json:"metadata"`
		Items []podResource `json:"items"`
	}
	err = decodeJSON(response.Body, 4<<20, &listed)
	response.Body.Close()
	cancel()
	if err != nil || listed.Metadata.ResourceVersion == "" || listed.Metadata.Continue != "" || listed.Items == nil {
		return errors.New("Kubernetes returned an incomplete engine pod list")
	}
	pods := make(map[string]podResource, len(listed.Items))
	for _, pod := range listed.Items {
		if !pod.Metadata.valid(w.namespace) {
			return errors.New("Kubernetes returned an invalid engine pod")
		}
		if _, exists := pods[pod.Metadata.UID]; exists {
			return errors.New("Kubernetes returned a duplicate pod UID")
		}
		pods[pod.Metadata.UID] = pod
	}

	watchContext, stopWatch := context.WithTimeout(ctx, 310*time.Second)
	defer stopWatch()
	response, err = w.get(watchContext, url.Values{
		"watch":               {"true"},
		"allowWatchBookmarks": {"true"},
		"resourceVersion":     {listed.Metadata.ResourceVersion},
		"timeoutSeconds":      {"300"},
	})
	if err != nil {
		return fmt.Errorf("engine pod watch: %w", err)
	}
	defer response.Body.Close()
	send(selectPod(pods))

	events := bufio.NewScanner(response.Body)
	events.Buffer(make([]byte, 8<<10), 4<<20)
	for events.Scan() {
		var event struct {
			Type   string          `json:"type"`
			Object json.RawMessage `json:"object"`
		}
		if json.Unmarshal(events.Bytes(), &event) != nil {
			return errors.New("Kubernetes returned an invalid pod watch event")
		}
		var pod podResource
		switch event.Type {
		case "ERROR":
			return errors.New("Kubernetes reported an engine pod watch error")
		case "BOOKMARK":
			if json.Unmarshal(event.Object, &pod) != nil || pod.Metadata.ResourceVersion == "" {
				return errors.New("Kubernetes returned an invalid pod watch bookmark")
			}
			// A healthy stream already resumes after this bookmark. On loss we relist.
			continue
		case "ADDED", "MODIFIED", "DELETED":
			if json.Unmarshal(event.Object, &pod) != nil || !pod.Metadata.valid(w.namespace) ||
				pod.Metadata.ResourceVersion == "" {
				return errors.New("Kubernetes returned an invalid watched engine pod")
			}
			if event.Type == "DELETED" {
				delete(pods, pod.Metadata.UID)
			} else {
				pods[pod.Metadata.UID] = pod
			}
			send(selectPod(pods))
		default:
			return errors.New("Kubernetes returned an unknown pod watch event")
		}
	}
	return errors.New("Kubernetes engine pod watch ended")
}

func (w *PodWatch) get(ctx context.Context, parameters url.Values) (*http.Response, error) {
	endpoint := w.api.JoinPath("api", "v1", "namespaces", w.namespace, "pods")
	if parameters == nil {
		parameters = make(url.Values)
	}
	parameters.Set("labelSelector", w.selector)
	endpoint.RawQuery = parameters.Encode()

	token, err := os.ReadFile(w.tokenPath)
	if err != nil || strings.TrimSpace(string(token)) == "" {
		return nil, errors.New("cannot read projected Kubernetes service account token")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("cannot create Kubernetes request")
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	request.Header.Set("Accept", "application/json")
	response, err := w.client.Do(request)
	if err != nil {
		return nil, errors.New("Kubernetes API connection failed")
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("Kubernetes API returned HTTP %d", response.StatusCode)
	}
	return response, nil
}

type podMetadata struct {
	Name              string  `json:"name"`
	Namespace         string  `json:"namespace"`
	UID               string  `json:"uid"`
	ResourceVersion   string  `json:"resourceVersion"`
	DeletionTimestamp *string `json:"deletionTimestamp"`
}

func (m podMetadata) valid(namespace string) bool {
	return m.Namespace == namespace && m.Name != "" && m.UID != ""
}

type podResource struct {
	Metadata podMetadata `json:"metadata"`
	Status   struct {
		PodIP      string         `json:"podIP"`
		Conditions []podCondition `json:"conditions"`
	} `json:"status"`
}

type podCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

func selectPod(pods map[string]podResource) *Pod {
	if len(pods) != 1 {
		return nil
	}
	for _, resource := range pods {
		address, err := netip.ParseAddr(resource.Status.PodIP)
		if err != nil || address.Zone() != "" || address.IsUnspecified() {
			return nil
		}
		pod := Pod{
			Namespace:   resource.Metadata.Namespace,
			Name:        resource.Metadata.Name,
			UID:         resource.Metadata.UID,
			Address:     address.String(),
			Terminating: resource.Metadata.DeletionTimestamp != nil,
		}
		for _, condition := range resource.Status.Conditions {
			if condition.Type == "Ready" {
				pod.Ready = condition.Status == "True"
				break
			}
		}
		return &pod
	}
	return nil
}

func retry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
