package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

// Prefix marks a value as encrypted so Decrypt can detect plaintext and
// return it unchanged. This makes the migration transparent: old rows have
// plaintext keys, new rows have prefix + base64 ciphertext.
const Prefix = "enc:"

// DeriveKey hashes a passphrase into a 32-byte AES-256 key. Using SHA-256 is
// sufficient here because the passphrase is already a high-entropy secret
// (the JWT secret); this is not password-based KDF territory.
func DeriveKey(passphrase string) []byte {
	h := sha256.Sum256([]byte(passphrase))
	return h[:]
}

// Encrypt AES-256-GCM encrypts plaintext and returns Prefix + base64(nonce +
// ciphertext). If key is empty the plaintext is returned unchanged so the
// feature is opt-in via config.
func Encrypt(key []byte, plaintext string) string {
	if len(key) == 0 || plaintext == "" {
		return plaintext
	}
	if IsEncrypted(plaintext) {
		return plaintext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return plaintext
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return plaintext
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plaintext
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return Prefix + base64.StdEncoding.EncodeToString(ciphertext)
}

// Decrypt reverses Encrypt. If the value is not prefixed (plaintext or
// empty) it is returned as-is so existing rows work without migration.
func Decrypt(key []byte, value string) string {
	if len(key) == 0 || value == "" || !IsEncrypted(value) {
		return value
	}
	raw, err := base64.StdEncoding.DecodeString(value[len(Prefix):])
	if err != nil {
		return value
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return value
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return value
	}
	if len(raw) < gcm.NonceSize() {
		return value
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return value
	}
	return string(plaintext)
}

// IsEncrypted returns true if value carries the encryption prefix.
func IsEncrypted(value string) bool {
	return len(value) > len(Prefix) && value[:len(Prefix)] == Prefix
}

// EncryptKey is a convenience for callers that store the key in config. A
// zero-length key disables encryption (plaintext passthrough).
type EncryptKey []byte

// FromPassphrase converts a config passphrase into an EncryptKey. A zero-length
// passphrase returns nil so encryption is disabled (plaintext passthrough).
func FromPassphrase(passphrase string) EncryptKey {
	if passphrase == "" {
		return nil
	}
	return DeriveKey(passphrase)
}
