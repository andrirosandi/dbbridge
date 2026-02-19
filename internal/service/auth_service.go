package service

import (
	"crypto/rand"
	"crypto/sha256"
	"dbbridge/internal/core"
	"encoding/hex"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo   core.UserRepository
	apiKeyRepo core.ApiKeyRepository
}

func NewAuthService(userRepo core.UserRepository, apiKeyRepo core.ApiKeyRepository) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		apiKeyRepo: apiKeyRepo,
	}
}

// SetupAdmin creates the first admin user, only allowed if no users exist
func (s *AuthService) SetupAdmin(username, password string) error {
	count, err := s.userRepo.CountUsers()
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("setup already completed")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.userRepo.CreateUser(username, string(hashedPassword))
	return err
}

// Authenticate checks credentials and returns user if valid
func (s *AuthService) Authenticate(username, password string) (*core.User, error) {
	user, err := s.userRepo.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("invalid credentials") // Don't leak if user exists
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

// API Key Management

func (s *AuthService) GenerateApiKey(userID int64, description string) (string, *core.ApiKey, error) {
	// Generate random 32-byte key
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, err
	}
	key := hex.EncodeToString(bytes)
	keyPrefix := key[:8]

	// Hash the key
	hasher := sha256.New()
	hasher.Write([]byte(key))
	keyHash := hex.EncodeToString(hasher.Sum(nil))

	apiKey := &core.ApiKey{
		UserID:      userID,
		KeyPrefix:   keyPrefix,
		KeyHash:     keyHash,
		Description: description,
		CreatedAt:   time.Now(),
		IsActive:    true,
	}

	if err := s.apiKeyRepo.Create(apiKey); err != nil {
		return "", nil, err
	}

	return key, apiKey, nil
}

func (s *AuthService) ValidateApiKey(key string, storedHash string) bool {
	hasher := sha256.New()
	hasher.Write([]byte(key))
	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash == storedHash
}

func (s *AuthService) VerifyApiKey(plainKey string) (*core.ApiKey, error) {
	// 1. Hash the key
	hasher := sha256.New()
	hasher.Write([]byte(plainKey))
	hash := hex.EncodeToString(hasher.Sum(nil))

	// 2. Lookup in DB
	apiKey, err := s.apiKeyRepo.GetByHash(hash)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, errors.New("invalid api key")
	}

	// 3. Update Last Used (Async or Sync? Sync for now)
	// Ignore error to not block auth
	_ = s.apiKeyRepo.UpdateLastUsed(apiKey.ID)

	return apiKey, nil
}

// HasUsers checks if system is set up
func (s *AuthService) HasUsers() (bool, error) {
	count, err := s.userRepo.CountUsers()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ResetPassword resets a user's password by username
func (s *AuthService) ResetPassword(username, newPassword string) error {
	user, err := s.userRepo.GetUserByUsername(username)
	if err != nil {
		return errors.New("user not found: " + username)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hashedPassword)
	return s.userRepo.Update(user)
}
