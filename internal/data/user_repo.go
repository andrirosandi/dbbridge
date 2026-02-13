package data

import (
	"database/sql"
	"dbbridge/internal/core"
	"time"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// CreateUser creates a new user with hashed password
func (r *UserRepo) CreateUser(username, passwordHash string) (*core.User, error) {
	res, err := r.db.Exec(`INSERT INTO users (username, password_hash, created_at, is_active) VALUES (?, ?, CURRENT_TIMESTAMP, 1)`, username, passwordHash)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &core.User{ID: id, Username: username, IsActive: true, CreatedAt: time.Now()}, nil
}

// GetUserByUsername retrieves a user by username
func (r *UserRepo) GetUserByUsername(username string) (*core.User, error) {
	var u core.User
	var isActive int
	err := r.db.QueryRow(`SELECT id, username, password_hash, is_active, created_at FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &isActive, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *UserRepo) GetByID(id int64) (*core.User, error) {
	var u core.User
	var isActive int
	err := r.db.QueryRow(`SELECT id, username, password_hash, is_active, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &isActive, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *UserRepo) GetAll() ([]core.User, error) {
	rows, err := r.db.Query(`SELECT id, username, is_active, created_at FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []core.User
	for rows.Next() {
		var u core.User
		var isActive int
		if err := rows.Scan(&u.ID, &u.Username, &isActive, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.IsActive = isActive == 1
		users = append(users, u)
	}
	return users, nil
}

func (r *UserRepo) Update(u *core.User) error {
	// Only update password if hash is not empty
	if u.PasswordHash != "" {
		_, err := r.db.Exec(`UPDATE users SET username=?, password_hash=?, is_active=? WHERE id=?`,
			u.Username, u.PasswordHash, u.IsActive, u.ID)
		return err
	}
	_, err := r.db.Exec(`UPDATE users SET username=?, is_active=? WHERE id=?`,
		u.Username, u.IsActive, u.ID)
	return err
}

func (r *UserRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE id=?`, id)
	return err
}

// CountUsers returns total number of users (useful for setup check)
func (r *UserRepo) CountUsers() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// TODO: Implement API Key methods when needed
func (r *UserRepo) CreateApiKey(userID int64, keyPrefix, keyHash string) (*core.ApiKey, error) {
	return nil, nil // Placeholder
}
func (r *UserRepo) GetApiKeyByHash(keyHash string) (*core.ApiKey, error) {
	return nil, nil // Placeholder
}
func (r *UserRepo) ValidateApiKey(plainKey string) (*core.User, error) {
	return nil, nil // Placeholder
}
