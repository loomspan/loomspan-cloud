package publication

import (
	"context"
	"errors"
)

var ErrSelectionNotFound = errors.New("application or publication revision was not found")

type Publication struct {
	ApplicationID string
	Namespace     string
	RevisionID    string
	Version       string
	PackDigest    string
}

type Selection struct {
	ApplicationID  string
	Namespace      string
	RevisionID     string
	PackDigest     string
	StagedRevision string
	TracePolicy    string
	Current        Publication
	SavedValues    map[string]string
}

type SelectionQuery struct {
	ApplicationID string
	// An empty RevisionID selects the current acknowledged publication for recovery.
	RevisionID string
}

type InstallPurpose string

const (
	InstallPublish InstallPurpose = "publish"
	InstallRestore InstallPurpose = "restore"
)

type InstallStatus string

const (
	InstallAcknowledged InstallStatus = "acknowledged"
	InstallNotSent      InstallStatus = "not-sent"
	InstallRejected     InstallStatus = "rejected"
	InstallUnconfirmed  InstallStatus = "unconfirmed"
)

type InstallAttempt struct {
	ID              string
	Purpose         InstallPurpose
	Publication     Publication
	ExpectedVersion string
	Settings        RuntimeSettings
	PodUID          string
	InstanceID      string
}

type Store interface {
	// OpenSelection calls neededNames with metadata from a coherent read snapshot.
	// The returned selection also contains the saved values for only those names.
	OpenSelection(ctx context.Context, query SelectionQuery, neededNames func(Selection) ([]string, error)) (Selection, error)
	CurrentPublication(ctx context.Context, applicationID string) (Publication, error)
	ListCurrentPublications(ctx context.Context) ([]Publication, error)
	StartInstall(ctx context.Context, attempt InstallAttempt) error
	CompleteInstall(ctx context.Context, attempt InstallAttempt, status InstallStatus) error
}
