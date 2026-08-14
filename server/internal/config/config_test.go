package config

import "testing"

// TestValidate_ProductionRequiresStrongSecrets verifies production refuses to
// boot with publicly known JWT secrets or without a dedicated ENCRYPTION_KEY.
func TestValidate_ProductionRequiresStrongSecrets(t *testing.T) {
	cases := []struct {
		name    string
		jwt     string
		enc     string
		wantErr bool
	}{
		{"empty jwt", "", "strong-encryption-key-123", true},
		{"default jwt", "change_me", "strong-encryption-key-123", true},
		{"placeholder jwt", "change_me_to_a_long_random_string_in_production", "strong-encryption-key-123", true},
		{"missing encryption key", "strong-jwt-secret-abcdef", "", true},
		{"weak encryption key", "strong-jwt-secret-abcdef", "change_me", true},
		{"both strong", "strong-jwt-secret-abcdef", "strong-encryption-key-123", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{
				Server: ServerConfig{Env: "production"},
				Auth:   AuthConfig{JWTSecret: tc.jwt, EncryptionKey: tc.enc},
			}
			err := c.validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestValidate_DevelopmentAllowsWeakSecrets verifies non-production keeps
// booting with defaults; hardening is resolveSecrets' job, not validate's.
func TestValidate_DevelopmentAllowsWeakSecrets(t *testing.T) {
	c := &Config{Server: ServerConfig{Env: "development"}, Auth: AuthConfig{JWTSecret: "change_me"}}
	if err := c.validate(); err != nil {
		t.Fatalf("validate() = %v, want nil in development", err)
	}
}

// TestResolveSecrets_ExplicitEncryptionKeyIndependent verifies the core of the
// key-separation fix: with ENCRYPTION_KEY set, the encryption key stays
// untouched regardless of the JWT secret, so rotating JWT_SECRET never makes
// stored credentials undecryptable.
func TestResolveSecrets_ExplicitEncryptionKeyIndependent(t *testing.T) {
	c := &Config{
		Server: ServerConfig{Env: "production"},
		Auth:   AuthConfig{JWTSecret: "jwt-rotated-value", EncryptionKey: "dedicated-encryption-key"},
	}
	c.resolveSecrets()
	if c.Auth.EncryptionKey != "dedicated-encryption-key" {
		t.Errorf("EncryptionKey = %q, want the explicitly configured value", c.Auth.EncryptionKey)
	}
	if c.Auth.JWTSecret != "jwt-rotated-value" {
		t.Errorf("JWTSecret = %q, want unchanged", c.Auth.JWTSecret)
	}
}

// TestResolveSecrets_FallbackCapturesOriginalJWT verifies the backward-compatible
// fallback: without ENCRYPTION_KEY the encryption key is derived from the
// ORIGINAL JWT secret, captured before dev-mode randomization. Existing
// ciphertexts stay decryptable and server/worker containers agree.
func TestResolveSecrets_FallbackCapturesOriginalJWT(t *testing.T) {
	c := &Config{
		Server: ServerConfig{Env: "development"},
		Auth:   AuthConfig{JWTSecret: "change_me"},
	}
	c.resolveSecrets()
	if c.Auth.EncryptionKey != "change_me" {
		t.Errorf("EncryptionKey = %q, want the original JWT secret", c.Auth.EncryptionKey)
	}
	if c.Auth.JWTSecret == "change_me" || c.Auth.JWTSecret == "" {
		t.Errorf("JWTSecret = %q, want a random per-boot replacement", c.Auth.JWTSecret)
	}
	if len(c.Auth.JWTSecret) != 64 { // randomHex(32) -> 64 hex chars
		t.Errorf("JWTSecret length = %d, want 64", len(c.Auth.JWTSecret))
	}
}

// TestResolveSecrets_RandomizedPerBoot verifies two consecutive boots with a
// weak default produce different signing keys.
func TestResolveSecrets_RandomizedPerBoot(t *testing.T) {
	a := &Config{Server: ServerConfig{Env: "development"}, Auth: AuthConfig{JWTSecret: ""}}
	a.resolveSecrets()
	b := &Config{Server: ServerConfig{Env: "development"}, Auth: AuthConfig{JWTSecret: ""}}
	b.resolveSecrets()
	if a.Auth.JWTSecret == b.Auth.JWTSecret {
		t.Error("two boots must not share the same random signing key")
	}
	// The encryption fallback must still be stable across boots.
	if a.Auth.EncryptionKey != b.Auth.EncryptionKey {
		t.Error("encryption fallback must be stable across boots")
	}
}

// TestResolveSecrets_StrongJWTUntouched verifies a configured strong JWT
// secret survives resolution in development and becomes the encryption
// fallback unchanged.
func TestResolveSecrets_StrongJWTUntouched(t *testing.T) {
	c := &Config{
		Server: ServerConfig{Env: "development"},
		Auth:   AuthConfig{JWTSecret: "my-own-strong-secret"},
	}
	c.resolveSecrets()
	if c.Auth.JWTSecret != "my-own-strong-secret" {
		t.Errorf("JWTSecret = %q, want unchanged", c.Auth.JWTSecret)
	}
	if c.Auth.EncryptionKey != "my-own-strong-secret" {
		t.Errorf("EncryptionKey = %q, want fallback to the JWT secret", c.Auth.EncryptionKey)
	}
}

// TestWorkerConcurrency_DefaultsAndEnvOverride verifies the worker pool
// sizing: positive env values apply, zero/negative values fall back to the
// defaults so a pool can never be silently disabled.
func TestWorkerConcurrency_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("WORKER_PIPELINE_CONCURRENCY", "12")
	t.Setenv("WORKER_AUX_CONCURRENCY", "0")
	cfg := defaults()
	overrideFromEnv(cfg)
	if cfg.Worker.PipelineConcurrency != 12 {
		t.Errorf("PipelineConcurrency = %d, want 12 from env", cfg.Worker.PipelineConcurrency)
	}
	if cfg.Worker.AuxConcurrency != 2 {
		t.Errorf("AuxConcurrency = %d, want default 2 (0 must not disable the pool)", cfg.Worker.AuxConcurrency)
	}
}
