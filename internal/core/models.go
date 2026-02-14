package core

import (
	"time"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // Added for Auth
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// ... (Other models remain same)
type ApiKey struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	KeyPrefix  string     `json:"key_prefix"`
	KeyHash    string     `json:"-"`
	IsActive   bool       `json:"is_active"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type DBConnection struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Driver              string `json:"driver"`
	ConnectionStringEnc string `json:"-"` // Encrypted
	IsActive            bool   `json:"is_active"`
}

type SavedQuery struct {
	ID                   int64   `json:"id"`
	Slug                 string  `json:"slug"`
	Description          string  `json:"description"`
	SQLText              string  `json:"sql_text"`
	ParamsConfig         string  `json:"params_config"` // JSON string
	IsActive             bool    `json:"is_active"`
	AllowedConnectionIDs []int64 `json:"allowed_connection_ids"` // Many-to-many
}

type AuditLog struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	UserID       int64     `json:"user_id"`
	ConnectionID int64     `json:"connection_id"`
	QueryID      int64     `json:"query_id"`
	DurationMs   int64     `json:"duration_ms"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message"`
}
