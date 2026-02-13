package data

import (
	"database/sql"
	"dbbridge/internal/core"
)

type AuditRepo struct {
	db *sql.DB
}

func NewAuditRepo(db *sql.DB) *AuditRepo {
	return &AuditRepo{db: db}
}

func (r *AuditRepo) Create(l *core.AuditLog) error {
	res, err := r.db.Exec(`INSERT INTO audit_logs (timestamp, user_id, connection_id, query_id, duration_ms, status, error_message) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.Timestamp, l.UserID, l.ConnectionID, l.QueryID, l.DurationMs, l.Status, l.ErrorMessage)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	l.ID = id
	return nil
}

func (r *AuditRepo) GetRecent(limit int) ([]core.AuditLog, error) {
	rows, err := r.db.Query(`SELECT id, timestamp, user_id, connection_id, query_id, duration_ms, status, error_message FROM audit_logs ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []core.AuditLog
	for rows.Next() {
		var l core.AuditLog
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.UserID, &l.ConnectionID, &l.QueryID, &l.DurationMs, &l.Status, &l.ErrorMessage); err != nil {
			return nil, err
		}

		// Adjust timezone if needed (SQLite stores UTC usually)
		l.Timestamp = l.Timestamp.Local()

		logs = append(logs, l)
	}
	return logs, nil
}
