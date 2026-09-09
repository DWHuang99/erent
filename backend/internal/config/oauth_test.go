package config

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestLoadOAuthEncryptionKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	t.Setenv("OAUTH_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
	actual, err := LoadOAuthEncryptionKey()
	if err != nil || !bytes.Equal(actual, key) {
		t.Fatal("valid encryption key rejected")
	}
	for _, value := range []string{"", "not-base64", base64.StdEncoding.EncodeToString([]byte("short"))} {
		t.Setenv("OAUTH_ENCRYPTION_KEY", value)
		if _, err := LoadOAuthEncryptionKey(); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
}
