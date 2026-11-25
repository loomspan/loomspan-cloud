package publication

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgres(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	if databaseURL == "" {
		return nil, errors.New("publication database URL is required")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("cannot configure publication database")
	}
	// Extended execution without prepared statements also works through the
	// Supabase transaction pooler. The driver still binds values separately.
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	db := stdlib.OpenDB(*config)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("cannot connect to publication database")
	}
	if _, err := db.ExecContext(ctx, "SELECT id FROM loomspan_cloud.applications WHERE false"); err != nil {
		db.Close()
		return nil, errors.New("publication database migration or access is unavailable")
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

const selectedApplicationSQL = `
	SELECT a.namespace, COALESCE(a.current_revision_id, ''), a.trace_persistence,
	       COALESCE(p.revision_id, ''), COALESCE(p.version, ''), COALESCE(p.pack_digest, '')
	FROM loomspan_cloud.applications a
	LEFT JOIN loomspan_cloud.publications p
	    ON p.application_id = a.id AND p.version = a.current_publication_version
	    AND p.status = 'acknowledged'
	WHERE a.id = $1`

func (s *PostgresStore) OpenSelection(ctx context.Context, query SelectionQuery, neededNames func(Selection) ([]string, error)) (Selection, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return Selection{}, err
	}
	defer tx.Rollback()
	selected := Selection{ApplicationID: query.ApplicationID}
	current := &selected.Current
	err = tx.QueryRowContext(ctx, selectedApplicationSQL, query.ApplicationID).Scan(
		&selected.Namespace, &selected.StagedRevision, &selected.TracePolicy,
		&current.RevisionID, &current.Version, &current.PackDigest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Selection{}, ErrSelectionNotFound
	}
	if err != nil {
		return Selection{}, err
	}
	if current.Version != "" {
		current.ApplicationID, current.Namespace = selected.ApplicationID, selected.Namespace
	}
	if query.RevisionID == "" {
		if current.Version == "" {
			return Selection{}, ErrSelectionNotFound
		}
		selected.RevisionID, selected.PackDigest = current.RevisionID, current.PackDigest
	} else {
		selected.RevisionID = query.RevisionID
		err := tx.QueryRowContext(ctx, `
			SELECT pack_digest FROM loomspan_cloud.stage_revisions WHERE application_id = $1 AND revision_id = $2
		`, selected.ApplicationID, selected.RevisionID).Scan(&selected.PackDigest)
		if errors.Is(err, sql.ErrNoRows) {
			return Selection{}, ErrSelectionNotFound
		}
		if err != nil {
			return Selection{}, err
		}
	}
	names, err := neededNames(selected)
	if err != nil {
		return Selection{}, err
	}
	selected.SavedValues, err = readSavedValues(ctx, tx, selected.ApplicationID, names)
	if err != nil {
		return Selection{}, err
	}
	return selected, tx.Commit()
}

func readSavedValues(ctx context.Context, tx *sql.Tx, applicationID string, names []string) (map[string]string, error) {
	values := make(map[string]string, len(names))
	lookup := make([]string, 0, len(names))
	for _, name := range names {
		if !strings.ContainsRune(name, 0) {
			// PostgreSQL text cannot contain NUL, so such a reference cannot
			// match any persisted name and is reported as missing.
			lookup = append(lookup, name)
		}
	}
	if len(lookup) == 0 {
		return values, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT name, value FROM loomspan_cloud.app_variables WHERE application_id = $1 AND name = ANY($2::text[])
	`, applicationID, lookup)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		values[name] = value
	}
	return values, rows.Err()
}

const currentPublicationsSQL = `
	SELECT a.id, a.namespace, p.revision_id, p.version, p.pack_digest
	FROM loomspan_cloud.applications a
	JOIN loomspan_cloud.publications p ON p.application_id = a.id AND p.version = a.current_publication_version
	    AND p.status = 'acknowledged'`

type rowScanner interface {
	Scan(...any) error
}

func scanPublication(row rowScanner) (Publication, error) {
	var result Publication
	err := row.Scan(&result.ApplicationID, &result.Namespace, &result.RevisionID, &result.Version, &result.PackDigest)
	return result, err
}

func (s *PostgresStore) CurrentPublication(ctx context.Context, applicationID string) (Publication, error) {
	p, err := scanPublication(s.db.QueryRowContext(ctx, currentPublicationsSQL+" WHERE a.id = $1", applicationID))
	if errors.Is(err, sql.ErrNoRows) {
		return Publication{}, ErrSelectionNotFound
	}
	return p, err
}

func (s *PostgresStore) ListCurrentPublications(ctx context.Context) ([]Publication, error) {
	rows, err := s.db.QueryContext(ctx, currentPublicationsSQL+" ORDER BY a.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Publication
	for rows.Next() {
		p, err := scanPublication(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *PostgresStore) StartInstall(ctx context.Context, attempt InstallAttempt) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p := attempt.Publication
	if attempt.Purpose == InstallPublish {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO loomspan_cloud.publications (application_id, version, revision_id, pack_digest, status)
			VALUES ($1, $2, $3, $4, 'started')
		`, p.ApplicationID, p.Version, p.RevisionID, p.PackDigest)
		if err != nil {
			return err
		}
	}
	settings, err := json.Marshal(attempt.Settings)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO loomspan_cloud.install_attempts (id, application_id, version, purpose, settings, pod_uid, instance_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'started')
	`, attempt.ID, p.ApplicationID, p.Version, attempt.Purpose, string(settings), attempt.PodUID, attempt.InstanceID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) CompleteInstall(ctx context.Context, attempt InstallAttempt, status InstallStatus) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p := attempt.Publication
	res, err := tx.ExecContext(ctx, `
		UPDATE loomspan_cloud.install_attempts SET status = $5, finished_at = now()
		WHERE id = $1 AND application_id = $2 AND version = $3 AND purpose = $4 AND status = 'started'
	`, attempt.ID, p.ApplicationID, p.Version, attempt.Purpose, status)
	if err := requireChanged(res, err, "started install attempt"); err != nil {
		return err
	}
	if attempt.Purpose == InstallPublish {
		res, err = tx.ExecContext(ctx, `
			UPDATE loomspan_cloud.publications SET status = $3
			WHERE application_id = $1 AND version = $2 AND status = 'started'
		`, p.ApplicationID, p.Version, status)
		if err := requireChanged(res, err, "started publication"); err != nil {
			return err
		}
		if status == InstallAcknowledged {
			res, err = tx.ExecContext(ctx, `
				UPDATE loomspan_cloud.applications SET current_publication_version = $2
				WHERE id = $1 AND current_publication_version IS NOT DISTINCT FROM NULLIF($3, '')
			`, p.ApplicationID, p.Version, attempt.ExpectedVersion)
			if err := requireChanged(res, err, "current publication pointer"); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func requireChanged(res sql.Result, err error, row string) error {
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%s is no longer current", row)
	}
	return nil
}
