package crypto

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := DeriveKey("secret-A")
	ct := Encrypt(key, "sk-my-api-key")
	if !IsEncrypted(ct) {
		t.Fatalf("expected %q to carry the encryption prefix", ct)
	}
	if got := Decrypt(key, ct); got != "sk-my-api-key" {
		t.Fatalf("Decrypt = %q, want original plaintext", got)
	}
}

// TestDecrypt_WrongKeyReturnsCiphertext documents the failure mode that
// motivated separating ENCRYPTION_KEY from JWT_SECRET: after a key rotation
// (wrong key), Decrypt silently returns the ciphertext, and callers would
// hand out "enc:AAAA..." as if it were the credential.
func TestDecrypt_WrongKeyReturnsCiphertext(t *testing.T) {
	ct := Encrypt(DeriveKey("secret-A"), "sk-my-api-key")
	if got := Decrypt(DeriveKey("secret-B"), ct); got != ct {
		t.Fatalf("Decrypt with wrong key = %q, want ciphertext unchanged", got)
	}
}

// TestPassthrough verifies the opt-in and migration semantics: a nil key
// disables encryption, empty values and legacy plaintext rows pass through,
// and encrypting an already-encrypted value is a no-op.
func TestPassthrough(t *testing.T) {
	key := DeriveKey("secret")
	if got := Encrypt(nil, "plain"); got != "plain" {
		t.Errorf("Encrypt with nil key = %q, want passthrough", got)
	}
	if got := Encrypt(key, ""); got != "" {
		t.Errorf("Encrypt empty = %q, want empty", got)
	}
	if got := Decrypt(key, "legacy-plaintext"); got != "legacy-plaintext" {
		t.Errorf("Decrypt plaintext = %q, want passthrough", got)
	}
	ct := Encrypt(key, "value")
	if got := Encrypt(key, ct); got != ct {
		t.Errorf("Encrypt twice = %q, want unchanged ciphertext", got)
	}
}
