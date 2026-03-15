package config

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv(SettingsEncryptionKeyEnv, "test-settings-key")

	ciphertext, err := Encrypt("super-secret", "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	if ciphertext == "super-secret" {
		t.Fatal("expected ciphertext to differ from plaintext")
	}

	plaintext, err := Decrypt(ciphertext, "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if plaintext != "super-secret" {
		t.Fatalf("expected decrypted plaintext, got %q", plaintext)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	t.Setenv(SettingsEncryptionKeyEnv, "test-settings-key")

	ciphertext, err := Encrypt("super-secret", "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	tampered := ciphertext[:len(ciphertext)-2] + "ab"

	if _, err := Decrypt(tampered, "BUTLER_OPENAI_API_KEY"); err == nil {
		t.Fatal("expected tampered ciphertext error")
	}
}

func TestEncryptRequiresEnvironmentKey(t *testing.T) {
	t.Setenv(SettingsEncryptionKeyEnv, "")

	_, err := Encrypt("super-secret", "BUTLER_OPENAI_API_KEY")
	if err == nil {
		t.Fatal("expected missing key error")
	}
	if err != ErrMissingSettingsEncryptionKey {
		t.Fatalf("expected ErrMissingSettingsEncryptionKey, got %v", err)
	}
}

func TestDecryptRejectsAssociatedDataMismatch(t *testing.T) {
	t.Setenv(SettingsEncryptionKeyEnv, "test-settings-key")

	ciphertext, err := Encrypt("super-secret", "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	_, err = Decrypt(ciphertext, "BUTLER_POSTGRES_URL")
	if err == nil {
		t.Fatal("expected associated data mismatch error")
	}
	if !strings.Contains(err.Error(), "decrypt ciphertext") {
		t.Fatalf("expected decrypt error, got %v", err)
	}
}
