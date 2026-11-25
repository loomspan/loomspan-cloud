package invocation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type PostgresGrants struct {
	db *sql.DB
}

func NewPostgresGrants(db *sql.DB) *PostgresGrants {
	return &PostgresGrants{db: db}
}

const userGrantSQL = `
	SELECT a.namespace, a.show_execution_details, to_json(g.roles)
	FROM loomspan_cloud.applications a
	JOIN loomspan_cloud.application_invoke_users g ON g.application_id = a.id
	WHERE a.id = $1 AND g.subject = $2`

const backendGrantSQL = `
	SELECT a.namespace, a.show_execution_details, '[]'::json
	FROM loomspan_cloud.applications a
	JOIN loomspan_cloud.application_invoke_backends g ON g.application_id = a.id
	WHERE a.id = $1 AND g.backend_id = $2`

func (s *PostgresGrants) Lookup(ctx context.Context, applicationID string, actor Actor) (Grant, error) {
	var query string
	switch actor.Kind {
	case DirectUser:
		query = userGrantSQL
	case CallingBackend:
		query = backendGrantSQL
	default:
		return Grant{}, ErrAccessDenied
	}
	var grant Grant
	var roles []byte
	err := s.db.QueryRowContext(ctx, query, applicationID, actor.ID).Scan(&grant.Namespace, &grant.ShowExecution, &roles)
	if errors.Is(err, sql.ErrNoRows) {
		return Grant{}, ErrAccessDenied
	}
	if err != nil || json.Unmarshal(roles, &grant.Roles) != nil {
		return Grant{}, ErrAuthorizationUnavailable
	}
	return grant, nil
}
