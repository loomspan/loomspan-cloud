package publication

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/loomspan/loomspan-cloud/management-backend/internal/engine"
)

var ErrPlacementUnavailable = errors.New("application execution is temporarily unavailable")

type acknowledgedInstall struct {
	version string
	target  observationAssociation
}

type Placement struct {
	Pod                    engine.Pod
	InstanceID             string
	Namespace              string
	Version                string
	GenerationID           string
	TracePersistencePolicy string
	ConnectionID           uint64
}

func (s *Service) Eligible(ctx context.Context, applicationID string) (Placement, error) {
	publication, err := s.store.CurrentPublication(ctx, applicationID)
	if err != nil {
		return Placement{}, ErrPlacementUnavailable
	}
	observed := s.observer.Current()
	installed, ok := observed.Confirms(publication.Namespace, publication.Version)
	if !ok {
		return Placement{}, ErrPlacementUnavailable
	}
	return Placement{
		Pod: observed.Pod, InstanceID: observed.InstanceID, ConnectionID: observed.ConnectionID,
		Namespace: publication.Namespace, Version: publication.Version, GenerationID: installed.GenerationID,
		TracePersistencePolicy: installed.TracePersistencePolicy,
	}, nil
}

type RecoveryFailure struct {
	ApplicationID string
	Version       string
	PodUID        string
	InstanceID    string
	Reason        string
}

// SetRecoveryReporter configures the recovery failure observer before Run.
func (s *Service) SetRecoveryReporter(report func(RecoveryFailure)) {
	if report != nil {
		s.report = report
	}
}

func logRecoveryFailure(failure RecoveryFailure) {
	slog.Warn("Could not recover publication", "application", failure.ApplicationID, "version", failure.Version,
		"podUID", failure.PodUID, "instanceID", failure.InstanceID, "reason", failure.Reason)
}

// Run restores durable intent once per authenticated observation association.
// Refreshes caused by acknowledged installs do not restart the reconcile pass.
func (s *Service) Run(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRefresh(ctx)
	}()
	defer func() { <-done }()

	var last engine.Projection
	for ctx.Err() == nil {
		changed := s.observer.Changes()
		target := s.observer.Current()
		var retry <-chan time.Time
		if _, ok := target.PublicationTarget(); ok && !sameAssociation(last, target) {
			if err := s.reconcile(ctx, target); err == nil {
				last = target
			} else if ctx.Err() == nil {
				s.report(RecoveryFailure{PodUID: target.Pod.UID, InstanceID: target.InstanceID, Reason: "cannot read durable publications"})
				retry = time.After(time.Second)
			}
		}
		select {
		case <-ctx.Done():
		case <-changed:
		case <-retry:
		}
	}
}

func (s *Service) runRefresh(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.refresh:
			if ctx.Err() == nil {
				s.observer.Refresh(ctx)
			}
		}
	}
}

func (s *Service) reconcile(ctx context.Context, target engine.Projection) error {
	intended, err := s.store.ListCurrentPublications(ctx)
	if err != nil {
		return err
	}
	for _, publication := range intended {
		current := s.observer.Current()
		if ctx.Err() != nil || !sameAssociation(target, current) || !current.Connected || current.Terminating {
			break
		}
		recovered, err := s.restore(ctx, target, publication.ApplicationID)
		if err != nil && ctx.Err() == nil {
			reason := "cannot read the current publication or its ZIP"
			var failure *PublishFailure
			if errors.As(err, &failure) {
				reason = string(failure.Outcome)
			}
			if recovered.Version == "" {
				recovered = publication
			}
			s.report(RecoveryFailure{
				ApplicationID: publication.ApplicationID, Version: recovered.Version,
				PodUID: target.Pod.UID, InstanceID: target.InstanceID, Reason: reason,
			})
		}
	}
	return nil
}

func (s *Service) restore(ctx context.Context, expected engine.Projection, applicationID string) (Publication, error) {
	release, err := s.gate.lock(ctx, applicationID)
	if err != nil {
		return Publication{}, err
	}
	defer release()
	target := s.observer.Current()
	if !sameAssociation(expected, target) || !target.Connected || target.Terminating {
		return Publication{}, nil
	}
	selected, pack, err := s.openSelection(ctx, SelectionQuery{ApplicationID: applicationID}, func(selection Selection) (bool, error) {
		return !s.alreadyInstalled(target, selection.Current), nil
	})
	if errors.Is(err, ErrSelectionNotFound) {
		return Publication{}, nil
	}
	if err != nil {
		return Publication{}, err
	}
	current := selected.Current
	if s.alreadyInstalled(target, current) {
		return current, nil
	}
	latest := s.observer.Current()
	if !sameAssociation(target, latest) || !latest.Connected || latest.Terminating || s.alreadyInstalled(latest, current) {
		return current, nil
	}
	settings, values, err := installInputs(selected, pack)
	if err != nil {
		return current, err
	}
	attempt := newAttempt(InstallRestore, current, current.Version, settings, target)
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return current, s.install(requestCtx, attempt, target, pack.data, values)
}

func (s *Service) alreadyInstalled(target engine.Projection, current Publication) bool {
	entry, present := target.Namespaces[current.Namespace]
	if present && entry.PublicationVersion != nil && *entry.PublicationVersion == current.Version {
		return true
	}
	// An owner install may have won the gate ahead of a waiting restore. It
	// has already installed this version even if Console has not caught up.
	// This receipt never qualifies a route, which requires a Console match.
	if value, ok := s.acknowledged.Load(current.ApplicationID); ok {
		ack := value.(acknowledgedInstall)
		return ack.version == current.Version && ack.target == association(target)
	}
	return false
}
