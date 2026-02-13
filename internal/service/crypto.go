package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// EncryptionService handles AES-256-GCM encryption/decryption
type EncryptionService struct {
	key []byte
}

// NewEncryptionService creates a new service with the provided key (must be 32 bytes/chars)
func NewEncryptionService(keyStr string) (*EncryptionService, error) {
	if len(keyStr) < 32 {
		return nil, errors.New("key must be at least 32 characters")
	}
	// Use exactly first 32 bytes for AES-256
	return &EncryptionService{key: []byte(keyStr)[:32]}, nil
}

// Encrypt encrypts plaintext using AES-GCM and returns base64 encoded string
func (s *EncryptionService) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64 encoded ciphertext
func (s *EncryptionService) Decrypt(cryptoText string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
