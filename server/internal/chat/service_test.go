package chat

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/pkg/crypto"
)

// fakeQuota records TryConsumeMessage calls so tests can assert exactly when
// (and how often) a daily quota unit is consumed.
type fakeQuota struct {
	calls int
}

func (f *fakeQuota) TryConsumeMessage(tenantID, userID string) (int, error) {
	f.calls++
	return 1, nil
}

func (f *fakeQuota) MessageUsage(tenantID, userID string) (int, int, error) {
	return 1, -1, nil
}

func newChatTestService(t *testing.T) (*Service, *gorm.DB, *fakeQuota) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// One connection so the in-memory database is shared across queries.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&Conversation{}, &Message{}, &llm.LLMModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	quota := &fakeQuota{}
	svc := NewService(NewRepo(db), nil, llm.NewRepo(db, crypto.FromPassphrase("test-key")), nil).
		WithMessageQuota(quota)
	return svc, db, quota
}

func insertTestConversation(t *testing.T, db *gorm.DB, tenantID, ownerID, convID, kbID string) {
	t.Helper()
	if err := db.Create(&Conversation{
		ID: convID, TenantID: tenantID, KbID: kbID, Title: "test", OwnerID: ownerID,
	}).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
}

func insertTestDefaultLLM(t *testing.T, db *gorm.DB, tenantID string) {
	t.Helper()
	if err := db.Create(&llm.LLMModel{
		ID: "llm1", TenantID: tenantID, Name: "test-llm", Endpoint: "http://localhost:1",
		Model: "test-model", IsDefault: true, Status: llm.StatusActive, OwnerID: "user1",
	}).Error; err != nil {
		t.Fatalf("create llm model: %v", err)
	}
}

// drainStream reads until the channel closes so spawned stream goroutines
// finish and leak no state across tests.
func drainStream(ch <-chan StreamReply) {
	for range ch {
	}
}

// Regression: a provider resolution failure (e.g. no LLM provider
// configured) must not consume a daily quota unit.
func TestTestStream_ProviderUnresolved_DoesNotConsumeQuota(t *testing.T) {
	svc, _, quota := newChatTestService(t)
	if _, err := svc.TestStream(context.Background(), "tenant1", "user1", "kb1", "hello"); err == nil {
		t.Fatal("expected error when no LLM provider is configured")
	}
	if quota.calls != 0 {
		t.Fatalf("quota consumed %d times on failed provider resolution; want 0", quota.calls)
	}
}

func TestStream_ProviderUnresolved_DoesNotConsumeQuota(t *testing.T) {
	svc, db, quota := newChatTestService(t)
	insertTestConversation(t, db, "tenant1", "user1", "conv1", "kb1")
	if _, err := svc.Stream(context.Background(), "tenant1", "user1", "conv1", SendInput{Message: "hi"}); err == nil {
		t.Fatal("expected error when no LLM provider is configured")
	}
	if quota.calls != 0 {
		t.Fatalf("quota consumed %d times on failed provider resolution; want 0", quota.calls)
	}
}

// When the provider resolves, exactly one quota unit is consumed.
func TestTestStream_ProviderResolved_ConsumesQuota(t *testing.T) {
	svc, db, quota := newChatTestService(t)
	insertTestDefaultLLM(t, db, "tenant1")
	ch, err := svc.TestStream(context.Background(), "tenant1", "user1", "kb1", "hello")
	if err != nil {
		t.Fatalf("TestStream: %v", err)
	}
	drainStream(ch)
	if quota.calls != 1 {
		t.Fatalf("quota consumed %d times; want 1", quota.calls)
	}
}

func TestStream_ProviderResolved_ConsumesQuotaAndSavesUserMessage(t *testing.T) {
	svc, db, quota := newChatTestService(t)
	insertTestConversation(t, db, "tenant1", "user1", "conv1", "kb1")
	insertTestDefaultLLM(t, db, "tenant1")
	ch, err := svc.Stream(context.Background(), "tenant1", "user1", "conv1", SendInput{Message: "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainStream(ch)
	if quota.calls != 1 {
		t.Fatalf("quota consumed %d times; want 1", quota.calls)
	}
	var msgs []Message
	if err := db.Where("tenant_id = ? AND conversation_id = ?", "tenant1", "conv1").Find(&msgs).Error; err != nil {
		t.Fatalf("load messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != RoleUser || msgs[0].Content != "hi" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
}
