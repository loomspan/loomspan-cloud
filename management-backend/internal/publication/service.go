package publication

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

type Actor struct {
	Issuer  string
	Subject string
}

type Permission string

const (
	PermissionValidate Permission = "validate-publication"
	PermissionPublish  Permission = "publish-publication"
)

type Authorizer interface {
	Authorize(context.Context, Actor, string, Permission) error
}

type AuthorizerFunc func(context.Context, Actor, string, Permission) error

func (f AuthorizerFunc) Authorize(ctx context.Context, actor Actor, applicationID string, permission Permission) error {
	return f(ctx, actor, applicationID, permission)
}

var ErrAccessDenied = errors.New("publication access denied")

type ValidationResult struct {
	RevisionID          string           `json:"revisionId"`
	RuntimeSettings     RuntimeSettings  `json:"runtimeSettings"`
	Valid               bool             `json:"valid"`
	Issues              []EngineIssue    `json:"issues"`
	Skills              []ValidatedSkill `json:"skills"`
	CloudIssues         []VariableIssue  `json:"cloudIssues"`
	PublicationBlockers []VariableIssue  `json:"publicationBlockers"`
}

type ValidationUnavailable struct {
	ShuttingDown bool
}

func (e *ValidationUnavailable) Error() string {
	if e.ShuttingDown {
		return "Validation is unavailable while the execution engine is shutting down. Retry when the engine is available."
	}
	return "Validation is unavailable. Please retry."
}

type StageConfirmation struct {
	ReviewedRevisionID string
	CurrentRevisionID  string
}

type PublishRequest struct {
	RevisionID   string
	Confirmation *StageConfirmation
}

type PublishResult struct {
	RevisionID       string
	Version          string
	AlreadyPublished bool
	Message          string
}

type PublishOutcome string

const (
	PublishConfirmationRequired PublishOutcome = "confirmation-required"
	PublishBlocked              PublishOutcome = "blocked"
	PublishNotSent              PublishOutcome = "not-sent"
	PublishRejected             PublishOutcome = "rejected"
	PublishUnconfirmed          PublishOutcome = "unconfirmed"
	PublishAcknowledgmentError  PublishOutcome = "acknowledgment-recording-error"
)

type PublishFailure struct {
	Outcome      PublishOutcome
	Message      string
	RevisionID   string
	Version      string
	Confirmation *StageConfirmation
	Issues       []VariableIssue
}

func (e *PublishFailure) Error() string {
	return e.Message
}

type engineGateway interface {
	validate(context.Context, engine.Projection, []byte, engineMetadata) (engineReport, *ValidationUnavailable)
	install(context.Context, engine.Projection, []byte, engineMetadata) installResponse
}

type Service struct {
	store        Store
	packs        PackReader
	observer     Observer
	engine       engineGateway
	authorizer   Authorizer
	gate         applicationGate
	acknowledged sync.Map
	refresh      chan struct{}
	report       func(RecoveryFailure)
}

// NewService constructs the shared publication workflow. A nil authorizer keeps
// owner operations closed while allowing the backend to recover durable intent.
func NewService(store Store, packs PackReader, observer Observer, client *EngineClient, authorizer Authorizer) *Service {
	return newService(store, packs, observer, client, authorizer)
}

func newService(store Store, packs PackReader, observer Observer, client engineGateway, authorizer Authorizer) *Service {
	return &Service{
		store: store, packs: packs, observer: observer, engine: client, authorizer: authorizer,
		refresh: make(chan struct{}, 1), report: logRecoveryFailure,
	}
}

func (s *Service) Validate(ctx context.Context, actor Actor, applicationID, revisionID string) (ValidationResult, error) {
	if err := s.authorize(ctx, actor, applicationID, PermissionValidate); err != nil {
		return ValidationResult{}, err
	}
	if revisionID == "" {
		return ValidationResult{}, ErrSelectionNotFound
	}
	selected, pack, err := s.openSelection(ctx, SelectionQuery{applicationID, revisionID}, nil)
	switch {
	case errors.Is(err, ErrSelectionNotFound):
		return ValidationResult{}, ErrSelectionNotFound
	case err != nil:
		return ValidationResult{}, &ValidationUnavailable{}
	}
	settings, err := EffectiveRuntimeSettings(selected.TracePolicy)
	if err != nil {
		return ValidationResult{}, &ValidationUnavailable{}
	}
	_, blockers := variableValues(pack.variables, selected.SavedValues)
	report, unavailable := s.engine.validate(ctx, s.observer.Current(), pack.data, engineMetadata{
		Namespace: selected.Namespace, RuntimeSettings: settings,
	})
	if unavailable != nil {
		return ValidationResult{}, unavailable
	}
	result := ValidationResult{
		RevisionID: revisionID, RuntimeSettings: settings, Valid: report.Valid && len(pack.issues) == 0,
		Issues: report.Issues, Skills: report.Skills, CloudIssues: pack.issues, PublicationBlockers: blockers,
	}
	if !result.Valid {
		result.Skills = []ValidatedSkill{}
	}
	return result, nil
}

func (s *Service) Publish(ctx context.Context, actor Actor, applicationID string, request PublishRequest) (PublishResult, error) {
	if err := s.authorize(ctx, actor, applicationID, PermissionPublish); err != nil {
		return PublishResult{}, err
	}
	if request.RevisionID == "" {
		return PublishResult{}, ErrSelectionNotFound
	}
	release, err := s.gate.lock(ctx, applicationID)
	if err != nil {
		return PublishResult{}, notSentFailure(request.RevisionID, false)
	}
	defer release()
	if err := s.authorize(ctx, actor, applicationID, PermissionPublish); err != nil {
		return PublishResult{}, err
	}
	var alreadyPublished bool
	selected, pack, err := s.openSelection(ctx, SelectionQuery{applicationID, request.RevisionID}, func(selected Selection) (bool, error) {
		if selected.Current.RevisionID == request.RevisionID {
			alreadyPublished = true
			return false, nil
		}
		compared := StageConfirmation{
			ReviewedRevisionID: request.RevisionID,
			CurrentRevisionID:  selected.StagedRevision,
		}
		if request.RevisionID != selected.StagedRevision && (request.Confirmation == nil || *request.Confirmation != compared) {
			return false, &PublishFailure{
				Outcome: PublishConfirmationRequired, Message: "Staging changed. Confirm publication of the reviewed revision.",
				RevisionID: request.RevisionID, Confirmation: &compared,
			}
		}
		return true, nil
	})
	if err != nil {
		var failure *PublishFailure
		if errors.Is(err, ErrSelectionNotFound) || errors.As(err, &failure) {
			return PublishResult{}, err
		}
		return PublishResult{}, notSentFailure(request.RevisionID, s.observer.Current().Terminating)
	}
	if alreadyPublished {
		return PublishResult{
			RevisionID: request.RevisionID, Version: selected.Current.Version, AlreadyPublished: true,
			Message: "This revision is already published.",
		}, nil
	}
	if ctx.Err() != nil {
		return PublishResult{}, notSentFailure(request.RevisionID, false)
	}
	target := s.observer.Current()
	if _, ok := target.PublicationTarget(); !ok {
		return PublishResult{}, notSentFailure(request.RevisionID, target.Terminating)
	}
	settings, values, err := installInputs(selected, pack)
	if err != nil {
		return PublishResult{}, err
	}
	publication := Publication{
		ApplicationID: selected.ApplicationID, Namespace: selected.Namespace,
		RevisionID: selected.RevisionID, PackDigest: selected.PackDigest, Version: randomID(),
	}
	attempt := newAttempt(InstallPublish, publication, selected.Current.Version, settings, target)
	if err := s.install(ctx, attempt, target, pack.data, values); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{RevisionID: selected.RevisionID, Version: publication.Version}, nil
}

func (s *Service) authorize(ctx context.Context, actor Actor, applicationID string, permission Permission) error {
	if s.authorizer == nil || blank(actor.Issuer) || blank(actor.Subject) || blank(applicationID) {
		return ErrAccessDenied
	}
	if err := s.authorizer.Authorize(ctx, actor, applicationID, permission); err != nil {
		return ErrAccessDenied
	}
	return nil
}

type openedPack struct {
	data      []byte
	variables variableDiscovery
	issues    []VariableIssue
}

func (s *Service) openSelection(ctx context.Context, query SelectionQuery, preflight func(Selection) (bool, error)) (Selection, openedPack, error) {
	var pack openedPack
	selected, err := s.store.OpenSelection(ctx, query, func(selection Selection) ([]string, error) {
		if preflight != nil {
			proceed, err := preflight(selection)
			if !proceed || err != nil {
				return nil, err
			}
		}
		original, err := s.packs.Read(selection.PackDigest)
		if err != nil {
			return nil, err
		}
		pack.data = original
		pack.variables, pack.issues = scanPack(original, selection.Namespace)
		return pack.variables.Names, nil
	})
	return selected, pack, err
}

func installInputs(selected Selection, pack openedPack) (RuntimeSettings, map[string]string, error) {
	settings, err := EffectiveRuntimeSettings(selected.TracePolicy)
	if err != nil {
		return RuntimeSettings{}, nil, &PublishFailure{Outcome: PublishBlocked, Message: err.Error(), RevisionID: selected.RevisionID}
	}
	values, blockers := variableValues(pack.variables, selected.SavedValues)
	if len(pack.issues)+len(blockers) > 0 {
		return RuntimeSettings{}, nil, &PublishFailure{
			Outcome: PublishBlocked, Message: "Publication has unresolved pack or variable findings.",
			RevisionID: selected.RevisionID, Issues: append(pack.issues, blockers...),
		}
	}
	return settings, values, nil
}

func (s *Service) install(ctx context.Context, attempt InstallAttempt, target engine.Projection, pack []byte, values map[string]string) error {
	p := attempt.Publication
	if err := s.store.StartInstall(ctx, attempt); err != nil {
		return notSentFailure(p.RevisionID, false)
	}
	response := s.engine.install(ctx, target, pack, engineMetadata{
		Namespace: p.Namespace, RuntimeSettings: attempt.Settings, PublicationVersion: p.Version, VariableValues: &values,
	})

	// The engine can acknowledge after cancellation reaches the caller.
	// Give the resulting database write its own bounded lifetime.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err := s.store.CompleteInstall(recordCtx, attempt, response.status)
	if response.status == InstallAcknowledged {
		if err != nil {
			return &PublishFailure{
				Outcome: PublishAcknowledgmentError, RevisionID: p.RevisionID, Version: p.Version,
				Message: "The engine acknowledged the installation, but recording its publication could not be confirmed.",
			}
		}
		s.acknowledged.Store(p.ApplicationID, acknowledgedInstall{p.Version, association(target)})
		s.requestRefresh()
		return nil
	}
	if err != nil {
		slog.Warn("Could not record publication failure", "application", p.ApplicationID,
			"version", p.Version, "status", response.status)
	}
	switch response.status {
	case InstallRejected:
		return &PublishFailure{Outcome: PublishRejected, Message: response.message, RevisionID: p.RevisionID, Version: p.Version}
	case InstallNotSent:
		return notSentFailure(p.RevisionID, response.shuttingDown)
	default:
		return &PublishFailure{
			Outcome: PublishUnconfirmed, RevisionID: p.RevisionID, Version: p.Version,
			Message: "The installation outcome is unconfirmed. It may still complete.",
		}
	}
}

func (s *Service) requestRefresh() {
	select {
	case s.refresh <- struct{}{}:
	default:
	}
}

func notSentFailure(revisionID string, terminating bool) *PublishFailure {
	message := "The installation was not sent. Retry when the execution engine is available."
	if terminating {
		message = "The installation was not sent because the execution engine is shutting down. Retry when the engine is available."
	}
	return &PublishFailure{Outcome: PublishNotSent, Message: message, RevisionID: revisionID}
}

func newAttempt(purpose InstallPurpose, publication Publication, expectedVersion string, settings RuntimeSettings, target engine.Projection) InstallAttempt {
	return InstallAttempt{
		ID: randomID(), Purpose: purpose, Publication: publication, ExpectedVersion: expectedVersion,
		Settings: settings, PodUID: target.Pod.UID, InstanceID: target.InstanceID,
	}
}

func randomID() string {
	var data [16]byte
	rand.Read(data[:])
	return hex.EncodeToString(data[:])
}

type applicationGate struct {
	mu    sync.Mutex
	lives map[string]*applicationLock
}

type applicationLock struct {
	token chan struct{}
	refs  int
}

func (g *applicationGate) lock(ctx context.Context, applicationID string) (func(), error) {
	g.mu.Lock()
	if g.lives == nil {
		g.lives = make(map[string]*applicationLock)
	}
	entry := g.lives[applicationID]
	if entry == nil {
		entry = &applicationLock{token: make(chan struct{}, 1)}
		g.lives[applicationID] = entry
	}
	entry.refs++
	g.mu.Unlock()
	drop := func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		entry.refs--
		if entry.refs == 0 {
			delete(g.lives, applicationID)
		}
	}
	select {
	case entry.token <- struct{}{}:
		return func() { <-entry.token; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}
