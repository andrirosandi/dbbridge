package data

import (
	"database/sql"
	"dbbridge/internal/core"
	"time"
)

type ApiKeyRepo struct {
	db *sql.DB
}

func NewApiKeyRepo(db *sql.DB) *ApiKeyRepo {
	return &ApiKeyRepo{db: db}
}

func (r *ApiKeyRepo) Create(key *core.ApiKey) error {
	query := `
		INSERT INTO api_keys (user_id, key_prefix, key_hash, description, created_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	res, err := r.db.Exec(query, key.UserID, key.KeyPrefix, key.KeyHash, key.Description, key.CreatedAt, key.IsActive)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	key.ID = id
	return nil
}

func (r *ApiKeyRepo) List() ([]core.ApiKey, error) {
	// For admin, listing all keys or maybe filtered by user.
	// For now, list all.
	query := `
		SELECT id, user_id, key_prefix, description, created_at, last_used_at, is_active
		FROM api_keys
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []core.ApiKey
	for rows.Next() {
		var k core.ApiKey
		var lastUsed sql.NullTime
		var desc sql.NullString
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyPrefix, &desc, &k.CreatedAt, &lastUsed, &k.IsActive); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		if desc.Valid {
			k.Description = desc.String
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (r *ApiKeyRepo) GetByHash(hash string) (*core.ApiKey, error) {
	query := `
		SELECT id, user_id, key_prefix, key_hash, description, created_at, last_used_at, is_active
		FROM api_keys
		WHERE key_hash = ? AND is_active = 1
	`
	row := r.db.QueryRow(query, hash)

	var k core.ApiKey
	var lastUsed sql.NullTime
	var desc sql.NullString
	if err := row.Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &desc, &k.CreatedAt, &lastUsed, &k.IsActive); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	if desc.Valid {
		k.Description = desc.String
	}
	return &k, nil
}

func (r *ApiKeyRepo) Revoke(id int64) error {
	query := `UPDATE api_keys SET is_active = 0 WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *ApiKeyRepo) UpdateLastUsed(id int64) error {
	query := `UPDATE api_keys SET last_used_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, time.Now(), id)
	return err
}
