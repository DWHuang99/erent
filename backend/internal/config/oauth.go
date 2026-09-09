package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// LoadOAuthEncryptionKey loads the persistent AES-256 key used by API token storage.
func LoadOAuthEncryptionKey() ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("OAUTH_ENCRYPTION_KEY")))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("OAUTH_ENCRYPTION_KEY must be a Base64-encoded 32-byte key when OAuth is enabled")
	}
	return key, nil
}
