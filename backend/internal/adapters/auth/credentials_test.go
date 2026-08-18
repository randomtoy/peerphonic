package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialCodecPersistsKeyAndProtectsPassword(t *testing.T) {
	t.Parallel()

	keyPath := filepath.Join(t.TempDir(), "auth.key")
	codec, err := NewCredentialCodec(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	hash, encrypted, err := codec.Encode("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(hash, []byte("correct horse")) || bytes.Contains(encrypted, []byte("correct horse")) {
		t.Fatal("stored credential contains plaintext password")
	}
	if err := codec.Verify(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if err := codec.Verify(hash, "wrong password"); err == nil {
		t.Fatal("Verify() accepted wrong password")
	}

	reopened, err := NewCredentialCodec(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := reopened.DecryptToken(encrypted)
	if err != nil || string(plaintext) != "correct horse battery staple" {
		t.Fatalf("DecryptToken() = %q, %v", plaintext, err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential key mode = %o, want 600", info.Mode().Perm())
	}
}
