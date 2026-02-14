package data

import (
	"database/sql"
	"dbbridge/internal/core"
	"fmt"
)

type AuditRepo struct {
	db *sql.DB
}

func NewAuditRepo(db *sql.DB) *AuditRepo {
	return &AuditRepo{db: db}
}

func (r *AuditRepo) Create(l *core.AuditLog) error {
	res, err := r.db.Exec(`INSERT INTO audit_logs (timestamp, user_id, api_key_id, connection_id, query_id, duration_ms, status, error_message, params) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.Timestamp, l.UserID, l.ApiKeyID, l.ConnectionID, l.QueryID, l.DurationMs, l.Status, l.ErrorMessage, l.Params)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	l.ID = id

	// Simple Retention Policy: Keep last 1000 logs
	// A more robust solution would be a background job, but this works for now.
	// Randomly trigger cleanup (e.g., 10% chance) to avoid overhead on every insert
	// or just do it. SQLite is fast enough for small tables.
	// Let's do it every time for "exactness" or maybe just strict limit?
	// DELETE FROM audit_logs WHERE id NOT IN (SELECT id FROM audit_logs ORDER BY id DESC LIMIT 1000)
	go func() {
		// Run in background to not block response
		limit := 1000 // TODO: Configurable
		_, _ = r.db.Exec(`DELETE FROM audit_logs WHERE id NOT IN (SELECT id FROM audit_logs ORDER BY id DESC LIMIT ?)`, limit)
	}()

	return nil
}

func (r *AuditRepo) GetRecent(limit int) ([]core.AuditLog, error) {
	query := `
		SELECT 
			a.id, a.timestamp, a.user_id, a.api_key_id, a.connection_id, a.query_id, a.duration_ms, a.status, a.error_message, a.params,
			k.key_prefix, k.description,
			c.name as connection_name,
			q.slug as query_slug
		FROM audit_logs a
		LEFT JOIN api_keys k ON a.api_key_id = k.id
		LEFT JOIN connections c ON a.connection_id = c.id
		LEFT JOIN queries q ON a.query_id = q.id
		ORDER BY a.timestamp DESC 
		LIMIT ?`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []core.AuditLog
	for rows.Next() {
		var l core.AuditLog
		var keyPrefix sql.NullString
		var keyDesc sql.NullString
		var connName sql.NullString
		var querySlug sql.NullString
		var params sql.NullString

		if err := rows.Scan(&l.ID, &l.Timestamp, &l.UserID, &l.ApiKeyID, &l.ConnectionID, &l.QueryID, &l.DurationMs, &l.Status, &l.ErrorMessage, &params, &keyPrefix, &keyDesc, &connName, &querySlug); err != nil {
			return nil, err
		}

		if params.Valid {
			l.Params = params.String
		}
		if connName.Valid {
			l.ConnectionName = connName.String
		}
		if querySlug.Valid {
			l.QuerySlug = querySlug.String
		}

		if keyPrefix.Valid {
			if keyDesc.Valid && keyDesc.String != "" {
				l.ApiKeyPrefix = fmt.Sprintf("%s... (%s)", keyPrefix.String, keyDesc.String)
			} else {
				l.ApiKeyPrefix = keyPrefix.String + "..."
			}
		}

		// Adjust timezone if needed (SQLite stores UTC usually)
		l.Timestamp = l.Timestamp.Local()

		logs = append(logs, l)
	}
	return logs, nil
}
