package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// Sealer encrypts and decrypts secrets with AES-GCM.
type Sealer struct {
	aead cipher.AEAD
}

// NewSealer builds a Sealer from the given key.
func NewSealer(key []byte) (*Sealer, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: creating cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: creating gcm: %w", err)
	}

	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext, prepending a fresh nonce so the result can
// be stored as it stands.
func (s *Sealer) Seal(plaintext, additionalData []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secrets: generating nonce: %w", err)
	}

	return s.aead.Seal(nonce, nonce, plaintext, additionalData), nil
}

// Open decrypts a value produced by Seal.
func (s *Sealer) Open(sealed, additionalData []byte) ([]byte, error) {
	nonceSize := s.aead.NonceSize()
	if len(sealed) < nonceSize+s.aead.Overhead() {
		return nil, errors.New("secrets: sealed value too short")
	}

	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]

	plaintext, err := s.aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("secrets: opening: %w", err)
	}

	return plaintext, nil
}
