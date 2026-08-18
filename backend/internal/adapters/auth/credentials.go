package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
)

const (
	credentialKeySize = 32
	credentialVersion = byte(1)
)

type CredentialCodec struct {
	aead cipher.AEAD
}

func NewCredentialCodec(keyPath string) (*CredentialCodec, error) {
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential encryption: %w", err)
	}
	return &CredentialCodec{aead: aead}, nil
}

func (c *CredentialCodec) Encode(password string) ([]byte, []byte, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("create credential nonce: %w", err)
	}
	sealed := make([]byte, 1, 1+len(nonce)+len(password)+c.aead.Overhead())
	sealed[0] = credentialVersion
	sealed = append(sealed, nonce...)
	sealed = c.aead.Seal(sealed, nonce, []byte(password), nil)
	return hash, sealed, nil
}

func (c *CredentialCodec) Verify(passwordHash []byte, password string) error {
	if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(password)); err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	return nil
}

func (c *CredentialCodec) DecryptToken(encrypted []byte) ([]byte, error) {
	nonceSize := c.aead.NonceSize()
	if len(encrypted) < 1+nonceSize+c.aead.Overhead() || encrypted[0] != credentialVersion {
		return nil, errors.New("invalid encrypted credential")
	}
	plaintext, err := c.aead.Open(nil, encrypted[1:1+nonceSize], encrypted[1+nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt credential: %w", err)
	}
	return plaintext, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != credentialKeySize {
			return nil, fmt.Errorf("credential key %q must contain %d bytes", path, credentialKeySize)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read credential key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create credential key directory: %w", err)
	}
	key = make([]byte, credentialKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate credential key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return loadOrCreateKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create credential key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		return nil, fmt.Errorf("write credential key: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, fmt.Errorf("sync credential key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close credential key: %w", err)
	}
	return key, nil
}
