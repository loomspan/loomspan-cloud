package publication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"sync/atomic"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

const (
	validatePath = "/internal/v1/validate"
	publishPath  = "/internal/v1/publish"
)

type Observer interface {
	Current() engine.Projection
	Changes() <-chan struct{}
	Refresh(context.Context) (engine.Projection, error)
}

type EngineClient struct {
	observer  Observer
	transport engine.PodClientFactory
}

func NewEngineClient(observer Observer, transport engine.PodClientFactory) *EngineClient {
	return &EngineClient{observer: observer, transport: transport}
}

type engineMetadata struct {
	Namespace          string             `json:"namespace"`
	RuntimeSettings    RuntimeSettings    `json:"runtimeSettings"`
	PublicationVersion string             `json:"publicationVersion,omitempty"`
	VariableValues     *map[string]string `json:"variableValues,omitempty"`
}

type EngineIssue struct {
	Severity   string `json:"severity"`
	SourceName string `json:"sourceName"`
	Message    string `json:"message"`
	SkillName  string `json:"skillName,omitempty"`
	FieldPath  string `json:"fieldPath,omitempty"`
}

type ValidatedSkill struct {
	Namespace string `json:"namespace"`
	LocalName string `json:"localName"`
	Kind      string `json:"kind"`
}

type engineReport struct {
	Valid  bool
	Issues []EngineIssue
	Skills []ValidatedSkill
}

func (c *EngineClient) validate(ctx context.Context, target engine.Projection, pack []byte, meta engineMetadata) (engineReport, *ValidationUnavailable) {
	response, failure := c.send(ctx, validatePath, target, pack, meta)
	if failure != nil {
		return engineReport{}, &ValidationUnavailable{ShuttingDown: failure.shuttingDown}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || mediaType(response) != "application/json" {
		return engineReport{}, &ValidationUnavailable{}
	}
	report, err := readValidationReport(response.Body, meta.Namespace)
	if err != nil {
		return engineReport{}, &ValidationUnavailable{}
	}
	return report, nil
}

func (c *EngineClient) install(ctx context.Context, target engine.Projection, pack []byte, meta engineMetadata) installResponse {
	response, failure := c.send(ctx, publishPath, target, pack, meta)
	if failure != nil {
		return *failure
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return installResponse{status: InstallAcknowledged}
	}
	if response.StatusCode < 400 || mediaType(response) != "text/plain" {
		return installResponse{status: InstallUnconfirmed}
	}
	message, err := io.ReadAll(response.Body)
	if err != nil {
		return installResponse{status: InstallUnconfirmed}
	}
	return installResponse{status: InstallRejected, message: string(message)}
}

type installResponse struct {
	status       InstallStatus
	message      string
	shuttingDown bool
}

func (c *EngineClient) send(ctx context.Context, endpoint string, target engine.Projection, pack []byte, meta engineMetadata) (*http.Response, *installResponse) {
	notSent := &installResponse{status: InstallNotSent}
	unknown := &installResponse{status: InstallUnconfirmed}
	pod, ok := target.PublicationTarget()
	if !ok {
		notSent.shuttingDown = target.Terminating
		return nil, notSent
	}
	client, base, err := c.transport.ForPod(pod)
	if err != nil {
		return nil, notSent
	}
	body, media, err := multipartPublication(pack, meta)
	if err != nil {
		client.CloseIdleConnections()
		return nil, notSent
	}
	var requestedConn, gotConn atomic.Bool
	trace := &httptrace.ClientTrace{
		GetConn: func(string) { requestedConn.Store(true) },
		GotConn: func(httptrace.GotConnInfo) { gotConn.Store(true) },
	}
	address := *base
	address.Path = endpoint
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, address.String(), body)
	if err != nil {
		client.CloseIdleConnections()
		return nil, notSent
	}
	req.Header.Set("Content-Type", media)
	req.GetBody = nil

	// Current acquires the observer's state lock. This is dispatch admission,
	// ordered with pod updates, after the request is ready and before any I/O.
	current := c.observer.Current()
	if ctx.Err() != nil || !sameAssociation(target, current) || !current.Connected || current.Terminating {
		client.CloseIdleConnections()
		notSent.shuttingDown = current.Terminating
		return nil, notSent
	}
	response, err := client.Do(req)
	if err != nil {
		client.CloseIdleConnections()
		if requestedConn.Load() && !gotConn.Load() {
			return nil, notSent
		}
		return nil, unknown
	}
	if response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		response.Body.Close()
		client.CloseIdleConnections()
		return nil, unknown
	}
	response.Body = &engineResponseBody{ReadCloser: response.Body, closeConnections: client.CloseIdleConnections}
	return response, nil
}

type engineResponseBody struct {
	io.ReadCloser
	closeConnections func()
}

func (b *engineResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.closeConnections()
	return err
}

type observationAssociation struct {
	podNamespace, podName, podUID, podAddress, instanceID string
	connectionID                                          uint64
}

func association(p engine.Projection) observationAssociation {
	return observationAssociation{p.Pod.Namespace, p.Pod.Name, p.Pod.UID, p.Pod.Address, p.InstanceID, p.ConnectionID}
}

func sameAssociation(a, b engine.Projection) bool {
	return association(a) == association(b)
}

func multipartPublication(pack []byte, meta engineMetadata) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadata, err := json.Marshal(meta)
	if err != nil {
		return nil, "", err
	}
	for _, item := range []struct {
		disposition string
		media       string
		content     []byte
	}{
		{`form-data; name="pack"; filename="publication.zip"`, "application/zip", pack},
		{`form-data; name="metadata"`, "application/json", metadata},
	} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", item.disposition)
		header.Set("Content-Type", item.media)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(item.content); err != nil {
			return nil, "", err
		}
	}
	return &body, writer.FormDataContentType(), writer.Close()
}

func mediaType(response *http.Response) string {
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return media
}

func readValidationReport(body io.Reader, namespace string) (engineReport, error) {
	var raw struct {
		Valid  *bool
		Issues *[]struct {
			Severity, SourceName, Message *string
			SkillName, FieldPath          string
		}
		Skills *[]ValidatedSkill
	}
	invalid := errors.New("invalid completed validation report")
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&raw); err != nil || raw.Valid == nil || raw.Issues == nil || raw.Skills == nil {
		return engineReport{}, invalid
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return engineReport{}, invalid
	}
	report := engineReport{Valid: *raw.Valid, Issues: make([]EngineIssue, 0, len(*raw.Issues)), Skills: *raw.Skills}
	hasError := false
	for _, issue := range *raw.Issues {
		if issue.Severity == nil || issue.SourceName == nil || issue.Message == nil ||
			(*issue.Severity != "ERROR" && *issue.Severity != "WARNING") {
			return engineReport{}, invalid
		}
		hasError = hasError || *issue.Severity == "ERROR"
		report.Issues = append(report.Issues, EngineIssue{*issue.Severity, *issue.SourceName, *issue.Message, issue.SkillName, issue.FieldPath})
	}
	if report.Valid == hasError || (!report.Valid && len(report.Skills) != 0) {
		return engineReport{}, invalid
	}
	for _, skill := range report.Skills {
		if skill.Namespace != namespace || skill.LocalName == "" || skill.Kind == "" {
			return engineReport{}, invalid
		}
	}
	return report, nil
}
