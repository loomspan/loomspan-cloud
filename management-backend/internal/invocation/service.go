package invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
	"github.com/loomspan/loomspan-cloud/management-backend/internal/publication"
)

const (
	MaxBodyBytes     = 20 * 1024 * 1024
	MaxUploadedFiles = 25
)

var (
	ErrBodyTooLarge             = errors.New("invocation body exceeds 20 MiB")
	ErrInvalidRequest           = errors.New("invalid invocation request")
	ErrAccessDenied             = errors.New("caller is not authorized to invoke the application")
	ErrAuthorizationUnavailable = errors.New("invocation authorization is temporarily unavailable")
	ErrUnavailable              = errors.New("application execution is temporarily unavailable")
	ErrOutcomeUnknown           = errors.New("invocation outcome is unknown")
)

type ActorKind uint8

const (
	DirectUser ActorKind = iota + 1
	CallingBackend
)

// Actor carries a verified identity from the selected authentication adapter.
// The caller's JSON cannot set either field.
type Actor struct {
	Kind ActorKind
	ID   string
}

type Grant struct {
	Namespace     string
	Roles         []string
	ShowExecution bool
}

type GrantReader interface {
	Lookup(context.Context, string, Actor) (Grant, error)
}

type PlacementResolver interface {
	Eligible(context.Context, string) (publication.Placement, error)
}

type Service struct {
	grants      GrantReader
	placements  PlacementResolver
	transport   engine.PodClientFactory
	cloudIssuer string
}

func NewService(grants GrantReader, placements PlacementResolver, transport engine.PodClientFactory, cloudIssuer string) (*Service, error) {
	if grants == nil || placements == nil || transport == nil || strings.TrimSpace(cloudIssuer) == "" {
		return nil, errors.New("invocation requires grants, placement, engine transport and a Cloud issuer")
	}
	return &Service{grants, placements, transport, cloudIssuer}, nil
}

type caller struct {
	Issuer  string   `json:"issuer"`
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

type endUser struct {
	ID    string   `json:"id"`
	Roles []string `json:"roles"`
}

type request struct {
	Namespace     string          `json:"namespace"`
	Skill         string          `json:"skill"`
	Input         json.RawMessage `json:"input"`
	Caller        caller          `json:"caller"`
	ExecutionView string          `json:"executionView,omitempty"`
}

// InvokeJSON handles the common JSON body before any transport-specific response.
// applicationID, localSkill and actor come from the authenticated adapter.
func (s *Service) InvokeJSON(ctx context.Context, actor Actor, applicationID, localSkill string, body io.Reader) (Response, error) {
	if actor.Kind != DirectUser && actor.Kind != CallingBackend || strings.TrimSpace(actor.ID) == "" ||
		(actor.Kind == CallingBackend && actor.ID == s.cloudIssuer) {
		return Response{}, ErrAccessDenied
	}
	if strings.TrimSpace(applicationID) == "" || strings.TrimSpace(localSkill) == "" || body == nil {
		return Response{}, ErrInvalidRequest
	}
	fields, err := readRequest(body)
	if err != nil {
		return Response{}, err
	}
	grant, err := s.grants.Lookup(ctx, applicationID, actor)
	if err != nil {
		if errors.Is(err, ErrAccessDenied) {
			return Response{}, ErrAccessDenied
		}
		return Response{}, ErrAuthorizationUnavailable
	}
	identity, err := s.authorizedCaller(actor, grant, fields["endUser"])
	if err != nil {
		return Response{}, err
	}
	message := request{Namespace: grant.Namespace, Skill: localSkill, Input: fields["input"], Caller: identity}
	if grant.ShowExecution {
		message.ExecutionView = "always"
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return Response{}, ErrInvalidRequest
	}
	return s.dispatch(ctx, applicationID, grant, payload)
}

func readRequest(body io.Reader) (map[string]json.RawMessage, error) {
	payload, err := io.ReadAll(io.LimitReader(body, MaxBodyBytes+1))
	if len(payload) > MaxBodyBytes {
		return nil, ErrBodyTooLarge
	}
	if err != nil || !utf8.Valid(payload) {
		return nil, ErrInvalidRequest
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, ErrInvalidRequest
	}
	input := bytes.TrimSpace(fields["input"])
	if len(input) == 0 || input[0] != '{' {
		return nil, ErrInvalidRequest
	}
	return fields, nil
}

func (s *Service) authorizedCaller(actor Actor, grant Grant, assertion json.RawMessage) (caller, error) {
	if actor.Kind == DirectUser {
		if !validRoles(grant.Roles) {
			return caller{}, ErrAuthorizationUnavailable
		}
		return caller{s.cloudIssuer, actor.ID, append([]string{}, grant.Roles...)}, nil
	}
	var claimed endUser
	if !validUnicodeEscapes(assertion) || json.Unmarshal(assertion, &claimed) != nil || strings.TrimSpace(claimed.ID) == "" ||
		claimed.Roles == nil || !validRoles(claimed.Roles) {
		return caller{}, ErrInvalidRequest
	}
	return caller{actor.ID, claimed.ID, claimed.Roles}, nil
}

// The assertion comes from a syntactically validated JSON document. Reject
// lone UTF-16 surrogates before encoding/json can replace them with U+FFFD.
func validUnicodeEscapes(encoded json.RawMessage) bool {
	for {
		slash := bytes.IndexByte(encoded, '\\')
		if slash < 0 {
			return true
		}
		encoded = encoded[slash+1:]
		if encoded[0] != 'u' {
			encoded = encoded[1:]
			continue
		}
		unit, _ := strconv.ParseUint(string(encoded[1:5]), 16, 16)
		encoded = encoded[5:]
		if unit < 0xd800 || unit > 0xdfff {
			continue
		}
		if unit >= 0xdc00 || !bytes.HasPrefix(encoded, []byte(`\u`)) {
			return false
		}
		low, _ := strconv.ParseUint(string(encoded[2:6]), 16, 16)
		if low < 0xdc00 || low > 0xdfff {
			return false
		}
		encoded = encoded[6:]
	}
}

func validRoles(roles []string) bool {
	for _, role := range roles {
		if strings.TrimSpace(role) == "" {
			return false
		}
	}
	return true
}

func (s *Service) dispatch(ctx context.Context, applicationID string, grant Grant, payload []byte) (Response, error) {
	target, err := s.placements.Eligible(ctx, applicationID)
	if err != nil || target.Namespace != grant.Namespace {
		return Response{}, ErrUnavailable
	}
	client, base, err := s.transport.ForPod(target.Pod)
	if err != nil || client == nil || base == nil || base.Scheme != "https" {
		return Response{}, ErrUnavailable
	}
	defer client.CloseIdleConnections()
	address := *base
	address.Path = "/internal/v1/invoke"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address.String(), bytes.NewReader(payload))
	if err != nil {
		return Response{}, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = nil
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// Recheck the watched pod, JVM, and durable version at dispatch admission.
	// The generation itself is always selected by Java when Framework admits.
	current, err := s.placements.Eligible(ctx, applicationID)
	if err != nil || ctx.Err() != nil || !samePlacement(target, current) {
		return Response{}, ErrUnavailable
	}
	response, err := noRedirect.Do(req)
	if err != nil {
		return Response{}, ErrOutcomeUnknown
	}
	defer response.Body.Close()
	if response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		return Response{}, ErrOutcomeUnknown
	}
	return projectResponse(response, grant.ShowExecution)
}

func samePlacement(a, b publication.Placement) bool {
	return a.Pod == b.Pod && a.InstanceID == b.InstanceID && a.ConnectionID == b.ConnectionID &&
		a.Namespace == b.Namespace && a.Version == b.Version
}
