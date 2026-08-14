package apikey

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ollmo/ollmo/pkg/errs"
)

// newAPIKeyDB opens an in-memory sqlite database and migrates the APIKey
// model. Each test gets its own fresh database.
func newAPIKeyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&APIKey{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// TestCreate_KeyFormat verifies the generated key starts with the "olk_"
// prefix and has the exact total length defined by KeyTotalLen (40 chars).
func TestCreate_KeyFormat(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	k, fullKey, err := svc.Create("tenant-1", "user-1", "test-key", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if k == nil {
		t.Fatal("expected non-nil APIKey")
	}

	if !strings.HasPrefix(fullKey, KeyPrefix) {
		t.Errorf("full key should start with %q, got %q", KeyPrefix, fullKey)
	}
	if len(fullKey) != KeyTotalLen {
		t.Errorf("full key length: want %d, got %d (key=%q)", KeyTotalLen, len(fullKey), fullKey)
	}
	// The hex suffix (after "olk_") must be 36 chars (18 bytes hex-encoded).
	suffix := fullKey[len(KeyPrefix):]
	if len(suffix) != 36 {
		t.Errorf("key suffix length: want 36, got %d", len(suffix))
	}
	for _, c := range suffix {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			t.Errorf("key suffix contains non-hex char %q in %q", c, suffix)
		}
	}
}

// TestCreateAndVerify verifies that a freshly created key can be verified,
// that the stored KeyHash is NOT the full key (only the hash is persisted),
// and that KeyPrefix matches the first 12 chars of the full key.
func TestCreateAndVerify(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	k, fullKey, err := svc.Create("tenant-1", "user-1", "my-key", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// KeyPrefix must match the first 12 chars of fullKey.
	wantPrefix := fullKey[:12]
	if k.KeyPrefix != wantPrefix {
		t.Errorf("KeyPrefix: want %q, got %q", wantPrefix, k.KeyPrefix)
	}

	// The stored hash must NOT equal the full key (only the sha256 hex is stored).
	if k.KeyHash == fullKey {
		t.Error("KeyHash must not equal the full key")
	}
	if k.KeyHash == "" {
		t.Error("KeyHash must be non-empty")
	}
	// sha256 hex is 64 chars.
	if len(k.KeyHash) != 64 {
		t.Errorf("KeyHash length: want 64 (sha256 hex), got %d", len(k.KeyHash))
	}

	// Verify the full key resolves to the stored record.
	verified, err := svc.Verify(fullKey)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified == nil {
		t.Fatal("expected non-nil verified key")
	}
	if verified.ID != k.ID {
		t.Errorf("verified ID: want %q, got %q", k.ID, verified.ID)
	}
	if verified.Status != StatusActive {
		t.Errorf("verified status: want %q, got %q", StatusActive, verified.Status)
	}
}

// TestVerify_InvalidKey verifies that a random string that does not match the
// key format (wrong prefix or wrong length) is rejected.
func TestVerify_InvalidKey(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	// Seed one key so the DB is not empty.
	_, _, err := svc.Create("tenant-1", "user-1", "k", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A totally random string with the wrong prefix and length.
	if _, err := svc.Verify("not-a-valid-key"); err == nil {
		t.Error("expected error for random string, got nil")
	}

	// A string with the right prefix but wrong length.
	short := KeyPrefix + "abc"
	if _, err := svc.Verify(short); err == nil {
		t.Error("expected error for short key, got nil")
	}

	// A 40-char string with the wrong prefix.
	wrongPrefix := "xxxx" + strings.Repeat("0", 36)
	if _, err := svc.Verify(wrongPrefix); err == nil {
		t.Error("expected error for wrong prefix, got nil")
	}
}

// TestVerify_Revoked verifies that a revoked key fails verification.
func TestVerify_Revoked(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	k, fullKey, err := svc.Create("tenant-1", "user-1", "to-revoke", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Sanity: the key verifies before revocation.
	if _, err := svc.Verify(fullKey); err != nil {
		t.Fatalf("Verify before revoke: %v", err)
	}

	if err := svc.Revoke("tenant-1", "user-1", k.ID, true); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	_, err = svc.Verify(fullKey)
	if err == nil {
		t.Fatal("expected error for revoked key, got nil")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T: %v", err, err)
	}
	if e.Code != errs.CodeUnauthorized {
		t.Errorf("expected CodeUnauthorized, got %v", e.Code)
	}
}

// TestVerify_Expired verifies that a key whose ExpiresAt is in the past fails
// verification.
func TestVerify_Expired(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	past := time.Now().Add(-1 * time.Hour)
	k, fullKey, err := svc.Create("tenant-1", "user-1", "expired", &past)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Confirm the expiry was persisted.
	if k.ExpiresAt == nil || !k.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expected ExpiresAt in the past, got %v", k.ExpiresAt)
	}

	_, err = svc.Verify(fullKey)
	if err == nil {
		t.Fatal("expected error for expired key, got nil")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T: %v", err, err)
	}
	if e.Code != errs.CodeUnauthorized {
		t.Errorf("expected CodeUnauthorized, got %v", e.Code)
	}
}

// TestVerify_NotExpired verifies that a key whose ExpiresAt is in the future
// passes verification. This guards against off-by-one errors in the expiry
// check.
func TestVerify_NotExpired(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	future := time.Now().Add(24 * time.Hour)
	k, fullKey, err := svc.Create("tenant-1", "user-1", "valid", &future)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if k.ExpiresAt == nil || !k.ExpiresAt.After(time.Now()) {
		t.Fatalf("expected ExpiresAt in the future, got %v", k.ExpiresAt)
	}

	verified, err := svc.Verify(fullKey)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified.ID != k.ID {
		t.Errorf("verified ID: want %q, got %q", k.ID, verified.ID)
	}
}

// TestCreate_EmptyName verifies that creating a key with an empty name is
// rejected with a BadRequest error.
func TestCreate_EmptyName(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	_, _, err := svc.Create("tenant-1", "user-1", "", nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T: %v", err, err)
	}
	if e.Code != errs.CodeBadRequest {
		t.Errorf("expected CodeBadRequest, got %v", e.Code)
	}
}

// TestVerify_UpdatesLastUsed verifies that a successful Verify call updates
// the LastUsedAt timestamp on the persisted record.
func TestVerify_UpdatesLastUsed(t *testing.T) {
	db := newAPIKeyDB(t)
	svc := NewService(NewRepo(db))

	k, fullKey, err := svc.Create("tenant-1", "user-1", "track-use", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// LastUsedAt should be nil right after creation.
	var before APIKey
	if err := db.First(&before, "id = ?", k.ID).Error; err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if before.LastUsedAt != nil {
		t.Errorf("LastUsedAt should be nil before first use, got %v", before.LastUsedAt)
	}

	beforeCall := time.Now().Add(-1 * time.Second)
	if _, err := svc.Verify(fullKey); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	var after APIKey
	if err := db.First(&after, "id = ?", k.ID).Error; err != nil {
		t.Fatalf("reload key after verify: %v", err)
	}
	if after.LastUsedAt == nil {
		t.Error("LastUsedAt should be set after Verify")
	}
	if after.LastUsedAt.Before(beforeCall) {
		t.Errorf("LastUsedAt %v should be after %v", after.LastUsedAt, beforeCall)
	}
}
