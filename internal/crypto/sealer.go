package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/sonolink/arbiterer/internal/config"
)

// Sealer encrypts and decrypts secrets with AES-256-GCM.
type Sealer struct {
	aead cipher.AEAD
}

// NewSealer builds a Sealer from the configured encryption key.
func NewSealer(cfg config.Crypto) (*Sealer, error) {
	block, err := aes.NewCipher(cfg.TokenKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: creating cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: creating gcm: %w", err)
	}

	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext, prepending a fresh nonce so the result can
// be stored as it stands.
func (s *Sealer) Seal(plaintext, additionalData []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generating nonce: %w", err)
	}

	return s.aead.Seal(nonce, nonce, plaintext, additionalData), nil
}

// Open decrypts a value produced by Seal.
func (s *Sealer) Open(sealed, additionalData []byte) ([]byte, error) {
	nonceSize := s.aead.NonceSize()
	if len(sealed) < nonceSize+s.aead.Overhead() {
		return nil, errors.New("crypto: sealed value too short")
	}

	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]

	plaintext, err := s.aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("crypto: opening: %w", err)
	}

	return plaintext, nil
}
