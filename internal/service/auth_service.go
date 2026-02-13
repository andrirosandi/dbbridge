package service

import (
	"dbbridge/internal/core"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo core.UserRepository
}

func NewAuthService(userRepo core.UserRepository) *AuthService {
	return &AuthService{
		userRepo: userRepo,
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

// HasUsers checks if system is set up
func (s *AuthService) HasUsers() (bool, error) {
	count, err := s.userRepo.CountUsers()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
