package core

// UserRepository defines storage operations for users and api keys
type UserRepository interface {
	CreateUser(username, passwordHash string) (*User, error)
	GetUserByUsername(username string) (*User, error)
	GetByID(id int64) (*User, error)
	GetAll() ([]User, error)
	Update(user *User) error
	Delete(id int64) error
	CountUsers() (int, error)
	CreateApiKey(userID int64, keyPrefix, keyHash string) (*ApiKey, error)
	GetApiKeyByHash(keyHash string) (*ApiKey, error)
	ValidateApiKey(plainKey string) (*User, error)
}

// ConnectionRepository defines storage operations for DB connections
type ConnectionRepository interface {
	Create(conn *DBConnection) error
	GetAll() ([]DBConnection, error)
	GetByID(id int64) (*DBConnection, error)
	Update(conn *DBConnection) error
	Delete(id int64) error
}

// QueryRepository defines storage operations for saved queries
type QueryRepository interface {
	Create(query *SavedQuery) error
	GetAll() ([]SavedQuery, error)
	GetByID(id int64) (*SavedQuery, error)
	GetBySlug(slug string) (*SavedQuery, error)
	Update(query *SavedQuery) error
	Delete(id int64) error
}

// AuditRepository defines storage operations for audit logs
type AuditRepository interface {
	Create(log *AuditLog) error
	GetRecent(limit int) ([]AuditLog, error)
}
