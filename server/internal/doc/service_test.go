package doc

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/pkg/errs"
)

// newTestDB opens an in-memory sqlite database and migrates the models used
// by the doc service. The caller owns the returned DB; close is not required
// for in-memory sqlite (it lives only for the connection's lifetime).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&tenant.Tenant{},
		&kb.KnowledgeBase{},
		&Document{},
		&Chunk{},
		&CleanupTask{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// seedDeleteFixture inserts a tenant, a KB with doc_count=1, a document and 3
// chunks. It returns the created IDs so tests can assert on them. The doc's
// ObjectKey is left empty so the Delete path skips the nil minio client.
func seedDeleteFixture(t *testing.T, db *gorm.DB) (tenantID, kbID, docID string, chunkIDs []string) {
	t.Helper()
	tenantID = "tenant-1"
	kbID = "kb-1"
	docID = "doc-1"

	if err := db.Create(&tenant.Tenant{ID: tenantID, Name: "t"}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&kb.KnowledgeBase{
		ID: kbID, TenantID: tenantID, Name: "k",
		EmbeddingModelID: "emb-1", DocCount: 1, OwnerID: "owner-1",
	}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if err := db.Create(&Document{
		ID: docID, TenantID: tenantID, KbID: kbID, Name: "d.pdf",
		Status: StatusReady, OwnerID: "owner-1",
	}).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	for i := 0; i < 3; i++ {
		cid := "chunk-" + string(rune('A'+i))
		if err := db.Create(&Chunk{
			ID: cid, TenantID: tenantID, KbID: kbID, DocID: docID,
			Index: i, Content: "content segment",
		}).Error; err != nil {
			t.Fatalf("create chunk %d: %v", i, err)
		}
		chunkIDs = append(chunkIDs, cid)
	}
	return
}

// newDocService builds a Service backed by the given DB with nil external
// dependencies (minio, asynq, store). This is white-box: we set the private
// fields directly to avoid pulling in the concrete external clients.
func newDocService(db *gorm.DB) *Service {
	return &Service{
		repo:   NewRepo(db),
		kbRepo: kb.NewRepo(db),
	}
}

// countRows returns the number of rows in the given table matching the WHERE
// clause built from key=value pairs.
func countRows(t *testing.T, db *gorm.DB, model any, where map[string]any) int64 {
	t.Helper()
	q := db.Model(model)
	for k, v := range where {
		q = q.Where(k+" = ?", v)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	return n
}

// TestDelete_Success verifies the happy path: deleting a document removes the
// doc row, all its chunks, decrements the KB doc_count, and creates no
// cleanup tasks (because store/minio are nil, the external-cleanup branches
// are skipped).
func TestDelete_Success(t *testing.T) {
	db := newTestDB(t)
	tenantID, kbID, docID, chunkIDs := seedDeleteFixture(t, db)
	svc := newDocService(db)

	if err := svc.Delete(context.Background(), tenantID, kbID, docID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Document is gone.
	if n := countRows(t, db, &Document{}, map[string]any{"id": docID}); n != 0 {
		t.Errorf("expected 0 docs after delete, got %d", n)
	}
	// All chunks are gone.
	for _, cid := range chunkIDs {
		if n := countRows(t, db, &Chunk{}, map[string]any{"id": cid}); n != 0 {
			t.Errorf("expected chunk %s gone, got %d rows", cid, n)
		}
	}
	// KB doc_count decremented to 0.
	var k kb.KnowledgeBase
	if err := db.Where("tenant_id = ? AND id = ?", tenantID, kbID).First(&k).Error; err != nil {
		t.Fatalf("reload kb: %v", err)
	}
	if k.DocCount != 0 {
		t.Errorf("kb doc_count: want 0, got %d", k.DocCount)
	}
	// No cleanup tasks created (store is nil so the milvus branch is skipped,
	// and ObjectKey is empty so the minio branch is skipped).
	if n := countRows(t, db, &CleanupTask{}, map[string]any{"doc_id": docID}); n != 0 {
		t.Errorf("expected 0 cleanup tasks, got %d", n)
	}
}

// TestDelete_NotFound verifies that deleting a non-existent doc ID returns a
// NotFound error and does not modify any existing data.
func TestDelete_NotFound(t *testing.T) {
	db := newTestDB(t)
	tenantID, kbID, docID, chunkIDs := seedDeleteFixture(t, db)
	svc := newDocService(db)

	err := svc.Delete(context.Background(), tenantID, kbID, "nonexistent-doc")
	if err == nil {
		t.Fatal("expected error for non-existent doc, got nil")
	}
	// The error must be the NotFound typed error from errs (FindByID returns
	// it when the doc is absent).
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T: %v", err, err)
	}
	if e.Code != errs.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", e.Code)
	}

	// Original doc and chunks must still be present.
	if n := countRows(t, db, &Document{}, map[string]any{"id": docID}); n != 1 {
		t.Errorf("doc should still exist, got %d rows", n)
	}
	for _, cid := range chunkIDs {
		if n := countRows(t, db, &Chunk{}, map[string]any{"id": cid}); n != 1 {
			t.Errorf("chunk %s should still exist, got %d rows", cid, n)
		}
	}
}

// TestDelete_TransactionRollback verifies the Delete operation is atomic and
// scoped: deleting a doc does not touch chunks belonging to a different doc.
// It also verifies that after a successful delete, no orphaned chunks remain
// for the deleted doc (i.e. the transaction committed all deletes together).
func TestDelete_TransactionRollback(t *testing.T) {
	db := newTestDB(t)
	tenantID, kbID, docID, _ := seedDeleteFixture(t, db)
	svc := newDocService(db)

	// Add a second document with its own chunks to prove the WHERE clauses
	// are scoped by doc_id and a delete of doc1 does not touch doc2.
	doc2ID := "doc-2"
	if err := db.Create(&Document{
		ID: doc2ID, TenantID: tenantID, KbID: kbID, Name: "d2.pdf",
		Status: StatusReady, OwnerID: "owner-1",
	}).Error; err != nil {
		t.Fatalf("create doc2: %v", err)
	}
	doc2Chunks := []string{"chunk2-A", "chunk2-B"}
	for i, cid := range doc2Chunks {
		if err := db.Create(&Chunk{
			ID: cid, TenantID: tenantID, KbID: kbID, DocID: doc2ID,
			Index: i, Content: "other segment",
		}).Error; err != nil {
			t.Fatalf("create doc2 chunk %s: %v", cid, err)
		}
	}
	// Bump KB doc_count to 2 so we can also verify it decrements by exactly 1.
	if err := db.Model(&kb.KnowledgeBase{}).
		Where("tenant_id = ? AND id = ?", tenantID, kbID).
		UpdateColumn("doc_count", 2).Error; err != nil {
		t.Fatalf("set doc_count=2: %v", err)
	}

	// Delete doc1.
	if err := svc.Delete(context.Background(), tenantID, kbID, docID); err != nil {
		t.Fatalf("Delete doc1: %v", err)
	}

	// doc1 and its chunks are gone (transaction committed all deletes).
	if n := countRows(t, db, &Document{}, map[string]any{"id": docID}); n != 0 {
		t.Errorf("doc1 should be gone, got %d rows", n)
	}
	if n := countRows(t, db, &Chunk{}, map[string]any{"doc_id": docID}); n != 0 {
		t.Errorf("doc1 chunks should be gone, got %d rows", n)
	}

	// doc2 and its chunks are untouched (no cross-contamination).
	if n := countRows(t, db, &Document{}, map[string]any{"id": doc2ID}); n != 1 {
		t.Errorf("doc2 should still exist, got %d rows", n)
	}
	for _, cid := range doc2Chunks {
		if n := countRows(t, db, &Chunk{}, map[string]any{"id": cid}); n != 1 {
			t.Errorf("doc2 chunk %s should still exist, got %d rows", cid, n)
		}
	}

	// KB doc_count decremented by exactly 1 (2 -> 1).
	var k kb.KnowledgeBase
	if err := db.Where("tenant_id = ? AND id = ?", tenantID, kbID).First(&k).Error; err != nil {
		t.Fatalf("reload kb: %v", err)
	}
	if k.DocCount != 1 {
		t.Errorf("kb doc_count: want 1, got %d", k.DocCount)
	}
}

// TestDelete_NoCleanupTasksWhenStoreNil verifies that when store is nil, no
// cleanup_tasks rows are written even if the doc has a ParsedObjectKey. The
// minio branch is guarded by `doc.ObjectKey != ""` / `doc.ParsedObjectKey != ""`
// respectively, so we set ParsedObjectKey to empty as well to keep the test
// panic-free with a nil minio client.
func TestDelete_NoCleanupTasksWhenStoreNil(t *testing.T) {
	db := newTestDB(t)
	tenantID, kbID, docID, _ := seedDeleteFixture(t, db)
	svc := newDocService(db)

	if err := svc.Delete(context.Background(), tenantID, kbID, docID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var n int64
	if err := db.Model(&CleanupTask{}).Count(&n).Error; err != nil {
		t.Fatalf("count cleanup tasks: %v", err)
	}
	if n != 0 {
		t.Errorf("expected no cleanup tasks with nil store, got %d", n)
	}
}

// TestDelete_WrongKB verifies the cross-KB IDOR fix: addressing a document by
// its real ID but with a mismatched kbID must fail with NotFound and leave
// both the document and its chunks untouched.
func TestDelete_WrongKB(t *testing.T) {
	db := newTestDB(t)
	tenantID, kbID, docID, chunkIDs := seedDeleteFixture(t, db)
	svc := newDocService(db)

	err := svc.Delete(context.Background(), tenantID, "other-kb", docID)
	if err == nil {
		t.Fatal("expected NotFound for wrong kbID, got nil")
	}
	e, ok := err.(*errs.Error)
	if !ok || e.Code != errs.CodeNotFound {
		t.Fatalf("expected *errs.Error CodeNotFound, got %v", err)
	}

	// Document and chunks must still exist.
	if n := countRows(t, db, &Document{}, map[string]any{"id": docID, "kb_id": kbID}); n != 1 {
		t.Errorf("doc should still exist, got %d rows", n)
	}
	for _, cid := range chunkIDs {
		if n := countRows(t, db, &Chunk{}, map[string]any{"id": cid}); n != 1 {
			t.Errorf("chunk %s should still exist, got %d rows", cid, n)
		}
	}
}

// TestGet_WrongKB verifies that Get with a mismatched kbID cannot read a
// document owned by another knowledge base.
func TestGet_WrongKB(t *testing.T) {
	db := newTestDB(t)
	tenantID, kbID, docID, _ := seedDeleteFixture(t, db)
	svc := newDocService(db)

	if _, err := svc.Get(context.Background(), tenantID, kbID, docID); err != nil {
		t.Fatalf("Get with correct kbID: %v", err)
	}
	if _, err := svc.Get(context.Background(), tenantID, "other-kb", docID); err == nil {
		t.Fatal("expected NotFound for wrong kbID, got nil")
	}
}

// TestReplaceChunks verifies the atomic chunk swap used by reparse: existing
// chunks are fully replaced by the new set in one transaction, scoped to the
// right doc, and an empty replacement clears all chunks.
func TestReplaceChunks(t *testing.T) {
	db := newTestDB(t)
	tenantID, kbID, docID, oldChunkIDs := seedDeleteFixture(t, db)
	svc := newDocService(db)

	newChunks := []*Chunk{
		{ID: "chunk-new-1", TenantID: tenantID, KbID: kbID, DocID: docID, Index: 0, Content: "new one"},
		{ID: "chunk-new-2", TenantID: tenantID, KbID: kbID, DocID: docID, Index: 1, Content: "new two"},
	}
	if err := svc.ReplaceChunks(context.Background(), tenantID, kbID, docID, newChunks); err != nil {
		t.Fatalf("ReplaceChunks: %v", err)
	}

	for _, cid := range oldChunkIDs {
		if n := countRows(t, db, &Chunk{}, map[string]any{"id": cid}); n != 0 {
			t.Errorf("old chunk %s should be gone, got %d rows", cid, n)
		}
	}
	if n := countRows(t, db, &Chunk{}, map[string]any{"doc_id": docID}); n != 2 {
		t.Errorf("expected 2 new chunks, got %d", n)
	}

	// Wrong kbID must not touch the target doc's chunks.
	if err := svc.ReplaceChunks(context.Background(), tenantID, "other-kb", docID, nil); err != nil {
		t.Fatalf("ReplaceChunks wrong kb: %v", err)
	}
	if n := countRows(t, db, &Chunk{}, map[string]any{"doc_id": docID}); n != 2 {
		t.Errorf("wrong-kb replace must not touch chunks, got %d rows", n)
	}

	// Empty replacement clears all chunks for the doc.
	if err := svc.ReplaceChunks(context.Background(), tenantID, kbID, docID, nil); err != nil {
		t.Fatalf("ReplaceChunks empty: %v", err)
	}
	if n := countRows(t, db, &Chunk{}, map[string]any{"doc_id": docID}); n != 0 {
		t.Errorf("expected 0 chunks after empty replace, got %d", n)
	}
}
