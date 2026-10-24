package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

func vaultAEAD(key string) (cipher.AEAD, error) {
	if len(key) < 32 {
		return nil, ErrVault
	}
	derived := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, ErrVault
	}
	return cipher.NewGCM(block)
}
func sealValue(key, value, aad string) (string, error) {
	a, err := vaultAEAD(key)
	if err != nil {
		return "", ErrVault
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", ErrVault
	}
	return base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(value), []byte(aad))), nil
}
func openValue(key, value, aad string) (string, error) {
	a, err := vaultAEAD(key)
	if err != nil {
		return "", ErrVault
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) < a.NonceSize() {
		return "", ErrVault
	}
	plain, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(aad))
	if err != nil {
		return "", ErrVault
	}
	return string(plain), nil
}
