package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const SettingsEncryptionKeyEnv = "BUTLER_SETTINGS_ENCRYPTION_KEY"

var ErrMissingSettingsEncryptionKey = errors.New("settings encryption key is not set")

func Encrypt(plaintext, associatedData string) (string, error) {
	key, err := settingsEncryptionKeyFromEnv()
	if err != nil {
		return "", err
	}
	return EncryptWithKey(key, plaintext, associatedData)
}

func Decrypt(ciphertext, associatedData string) (string, error) {
	key, err := settingsEncryptionKeyFromEnv()
	if err != nil {
		return "", err
	}
	return DecryptWithKey(key, ciphertext, associatedData)
}

func EncryptWithKey(secretKey, plaintext, associatedData string) (string, error) {
	aead, err := newSettingsAEAD(secretKey)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := aead.Seal(nil, nonce, []byte(plaintext), []byte(associatedData))
	payload := append(nonce, sealed...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func DecryptWithKey(secretKey, ciphertext, associatedData string) (string, error) {
	aead, err := newSettingsAEAD(secretKey)
	if err != nil {
		return "", err
	}

	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(payload) < aead.NonceSize() {
		return "", fmt.Errorf("ciphertext is too short")
	}

	nonce := payload[:aead.NonceSize()]
	sealed := payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, []byte(associatedData))
	if err != nil {
		return "", fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return string(plaintext), nil
}

func settingsEncryptionKeyFromEnv() (string, error) {
	key := strings.TrimSpace(os.Getenv(SettingsEncryptionKeyEnv))
	if key == "" {
		return "", ErrMissingSettingsEncryptionKey
	}
	return key, nil
}

func newSettingsAEAD(secretKey string) (cipher.AEAD, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, ErrMissingSettingsEncryptionKey
	}
	derived := sha256.Sum256([]byte(secretKey))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM cipher: %w", err)
	}
	return aead, nil
}
