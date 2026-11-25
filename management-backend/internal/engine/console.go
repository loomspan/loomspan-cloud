package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	consolePath    = "/_loomspan/observability/v1"
	consoleKey     = "X-loomspan-Api-Key"
	instanceHeader = "X-loomspan-Instance-Id"
)

var instanceUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type NamespaceObservation struct {
	Namespace              string  `json:"namespace"`
	GenerationID           string  `json:"generationId"`
	PublicationVersion     *string `json:"publicationVersion"`
	TracePersistencePolicy string  `json:"tracePersistencePolicy"`
}

type Snapshot struct {
	InstanceID             string
	Namespaces             map[string]NamespaceObservation
	TracePersistencePolicy string
}

// PodClientFactory establishes a service-authenticated client for the selected pod.
type PodClientFactory interface {
	ForPod(Pod) (*http.Client, *url.URL, error)
}

type Console struct {
	transport PodClientFactory
	key       string
	timeout   time.Duration
}

func NewConsole(transport PodClientFactory, key string) (*Console, error) {
	if transport == nil || len(key) < 32 || len(key) > 512 {
		return nil, errors.New("Console requires a pod transport and valid API key")
	}
	for _, character := range key {
		if character < '!' || character > '~' {
			return nil, errors.New("Console API key must contain printable non-space ASCII")
		}
	}
	return &Console{transport: transport, key: key, timeout: 10 * time.Second}, nil
}

func (c *Console) connect(pod Pod) (*consoleConnection, error) {
	client, base, err := c.transport.ForPod(pod)
	if err != nil {
		return nil, err
	}
	return &consoleConnection{client: client, base: base, key: c.key, timeout: c.timeout}, nil
}

type consoleConnection struct {
	client  *http.Client
	base    *url.URL
	key     string
	timeout time.Duration
}

func (c *consoleConnection) snapshot(ctx context.Context) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.get(ctx, "/instance", nil, "application/json")
	if err != nil {
		return Snapshot{}, err
	}
	defer response.Body.Close()

	var body struct {
		InstanceID             string                 `json:"instanceId"`
		Namespaces             []NamespaceObservation `json:"namespaces"`
		TracePersistencePolicy string                 `json:"tracePersistencePolicy"`
	}
	header := response.Header.Values(instanceHeader)
	if decodeJSON(response.Body, 4<<20, &body) != nil || len(header) != 1 || header[0] != body.InstanceID ||
		!instanceUUID.MatchString(body.InstanceID) || body.Namespaces == nil ||
		!validPolicy(body.TracePersistencePolicy) {
		return Snapshot{}, errors.New("invalid authenticated Console instance snapshot")
	}
	namespaces := make(map[string]NamespaceObservation, len(body.Namespaces))
	for _, namespace := range body.Namespaces {
		published := namespace.PublicationVersion != nil
		if !validID(namespace.Namespace) || !validID(namespace.GenerationID) ||
			(published && *namespace.PublicationVersion == "") ||
			((published || namespace.TracePersistencePolicy != "") && !validPolicy(namespace.TracePersistencePolicy)) {
			return Snapshot{}, errors.New("invalid namespace in Console instance snapshot")
		}
		if _, exists := namespaces[namespace.Namespace]; exists {
			return Snapshot{}, errors.New("duplicate namespace in Console instance snapshot")
		}
		namespaces[namespace.Namespace] = namespace
	}
	return Snapshot{body.InstanceID, namespaces, body.TracePersistencePolicy}, nil
}

func validID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

func validPolicy(value string) bool {
	return value == "NEVER" || value == "ONERROR" || value == "ALWAYS"
}

func (c *consoleConnection) baseline(ctx context.Context, instance string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.get(ctx, "/active-executions", url.Values{"pageSize": {"1"}}, "application/json")
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var body struct {
		ResumeCursor string `json:"resumeCursor"`
	}
	if !matchesInstance(response, instance) || decodeJSON(response.Body, 4<<20, &body) != nil {
		return "", errors.New("Console baseline belongs to a different JVM or is malformed")
	}
	if _, err := parseCursor(body.ResumeCursor); err != nil {
		return "", errors.New("invalid Console resume cursor")
	}
	return body.ResumeCursor, nil
}

func (c *consoleConnection) get(ctx context.Context, route string, parameters url.Values, accept string) (*http.Response, error) {
	endpoint := *c.base
	endpoint.Path = consolePath + route
	endpoint.RawQuery = parameters.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("cannot construct Console request")
	}
	request.Header.Set(consoleKey, c.key)
	request.Header.Set("Accept", accept)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Console %s connection failed", route)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("Console %s returned HTTP %d", route, response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != accept {
		response.Body.Close()
		return nil, fmt.Errorf("Console %s returned an invalid content type", route)
	}
	return response, nil
}

func matchesInstance(response *http.Response, instance string) bool {
	header := response.Header.Values(instanceHeader)
	return len(header) == 1 && header[0] == instance
}

func (c *consoleConnection) stream(ctx context.Context, instance string, admitted func()) error {
	cursor, err := c.baseline(ctx, instance)
	if err != nil {
		return err
	}
	streamContext, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(c.timeout, cancel)
	defer timer.Stop()
	response, err := c.get(streamContext, "/activity", url.Values{
		"instanceId":  {instance},
		"afterCursor": {cursor},
	}, "text/event-stream")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if !matchesInstance(response, instance) {
		return errors.New("Console activity stream belongs to a different JVM")
	}

	reader := bufio.NewScanner(response.Body)
	reader.Buffer(make([]byte, 8<<10), 1<<20)
	frame, err := activityFrame(reader)
	var handshake struct {
		InstanceID  string `json:"instanceId"`
		AfterCursor string `json:"afterCursor"`
	}
	if err != nil || frame.event != "handshake" || frame.id != "" ||
		json.Unmarshal(frame.data, &handshake) != nil || handshake.InstanceID != instance ||
		handshake.AfterCursor != cursor {
		return errors.New("invalid Console activity stream handshake")
	}
	if !timer.Stop() {
		return errors.New("Console activity stream handshake timed out")
	}
	admitted()
	last, _ := parseCursor(cursor)
	for {
		frame, err = activityFrame(reader)
		if err != nil {
			return err
		}
		var activity struct {
			InstanceID string `json:"instanceId"`
			Cursor     string `json:"cursor"`
		}
		next, cursorErr := parseCursor(frame.id)
		if frame.event != "activity" || cursorErr != nil || next <= last ||
			json.Unmarshal(frame.data, &activity) != nil || activity.InstanceID != instance ||
			activity.Cursor != frame.id {
			return errors.New("invalid Console activity stream frame")
		}
		last = next
	}
}

type sseFrame struct {
	event string
	id    string
	data  []byte
}

func activityFrame(reader *bufio.Scanner) (sseFrame, error) {
	var frame sseFrame
	for reader.Scan() {
		line := reader.Text()
		if line == "" {
			if frame.event == "" && frame.id == "" && frame.data == nil {
				continue
			}
			return frame, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch {
		case found && field == "id" && frame.id == "" && value != "":
			frame.id = value
		case found && field == "event" && frame.event == "" && value != "":
			frame.event = value
		case found && field == "data" && frame.data == nil:
			frame.data = []byte(value)
		default:
			return sseFrame{}, errors.New("invalid Console SSE field")
		}
	}
	return sseFrame{}, errors.New("Console activity stream ended or exceeded its frame limit")
}

func parseCursor(cursor string) (uint64, error) {
	return strconv.ParseUint(cursor, 10, 63)
}

func decodeJSON(body io.Reader, limit int64, value any) error {
	payload, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(payload)) > limit {
		return errors.New("response exceeded its size limit")
	}
	return json.Unmarshal(payload, value)
}
