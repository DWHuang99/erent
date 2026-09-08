package security

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestEncryptionRoundTripAndAuthentication(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	for _, plain := range [][]byte{nil, []byte("access-token")} {
		first, err := Encrypt(key, plain)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Encrypt(key, plain)
		if err != nil || bytes.Equal(first, second) {
			t.Fatal("encryption must use a fresh nonce")
		}
		decoded, err := Decrypt(key, first)
		if err != nil || !bytes.Equal(decoded, plain) {
			t.Fatal("decrypt did not restore token")
		}
		if _, err := Decrypt([]byte("abcdef0123456789abcdef0123456789"), first); err == nil {
			t.Fatal("wrong key accepted")
		}
		ciphertext, err := base64.StdEncoding.DecodeString(string(first))
		if err != nil {
			t.Fatal(err)
		}
		ciphertext[len(ciphertext)-1] ^= 1
		if _, err := Decrypt(key, []byte(base64.StdEncoding.EncodeToString(ciphertext))); err == nil {
			t.Fatal("tampered ciphertext accepted")
		}
	}
	if _, err := Encrypt(nil, []byte("token")); err == nil {
		t.Fatal("missing key accepted")
	}
	for _, malformed := range []string{"!", "", "YQ=="} {
		if _, err := Decrypt(key, []byte(malformed)); err == nil {
			t.Fatal("malformed ciphertext accepted")
		}
	}
}
