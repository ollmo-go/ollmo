package doc

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"

	"ollmo/ollmo/pkg/vector"
)

// CleanupMaxAttempts caps how many times a task is retried before it is
// marked as "exhausted" and left for manual intervention.
const CleanupMaxAttempts = 5

// CleanupRetryInterval is the delay between sweep passes.
const CleanupRetryInterval = 2 * time.Minute

// CleanupBackoff is the exponential backoff base between retry attempts for
// a single task.
var CleanupBackoff = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
}

// CleanupWorker sweeps cleanup_tasks for pending retries and re-attempts
// the failed Milvus/MinIO deletion. It runs as a background goroutine
// alongside the Asynq worker process.
type CleanupWorker struct {
	db     *gorm.DB
	store  *vector.Store
	minio  *minio.Client
	bucket string
}

func NewCleanupWorker(db *gorm.DB, store *vector.Store, mc *minio.Client, bucket string) *CleanupWorker {
	return &CleanupWorker{db: db, store: store, minio: mc, bucket: bucket}
}

// Start runs the sweep loop until ctx is cancelled. It is meant to be
// called as `go worker.Start(ctx)` from the worker process.
func (w *CleanupWorker) Start(ctx context.Context) {
	log.Printf("[cleanup] worker started, interval=%s", CleanupRetryInterval)
	ticker := time.NewTicker(CleanupRetryInterval)
	defer ticker.Stop()
	// Run once immediately so we don't wait for the first tick on startup.
	w.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[cleanup] worker stopped")
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce queries pending tasks whose retry time has come and attempts
// the cleanup. Safe to call concurrently; each task is claimed with an
// atomic status update before processing.
func (w *CleanupWorker) RunOnce(ctx context.Context) {
	now := time.Now()
	var tasks []CleanupTask
	err := w.db.Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", "pending", now).
		Order("created_at ASC").
		Limit(50).
		Find(&tasks).Error
	if err != nil {
		log.Printf("[cleanup] query failed: %v", err)
		return
	}
	if len(tasks) == 0 {
		return
	}
	log.Printf("[cleanup] processing %d pending tasks", len(tasks))
	for _, t := range tasks {
		w.retryTask(ctx, t)
	}
}

func (w *CleanupWorker) retryTask(ctx context.Context, t CleanupTask) {
	if t.Attempts >= t.MaxAttempts {
		w.markExhausted(t)
		return
	}

	err := w.execute(ctx, t)
	if err == nil {
		// Success: mark as done.
		w.db.Model(&CleanupTask{}).Where("id = ?", t.ID).Updates(map[string]any{
			"status":   "done",
			"error":    "",
			"attempts": t.Attempts + 1,
		})
		log.Printf("[cleanup] task %s done (kind=%s doc=%s)", t.ID, t.Kind, t.DocID)
		return
	}

	// Failure: increment attempts and schedule next retry with backoff.
	nextAttempt := t.Attempts + 1
	var nextRetry time.Time
	idx := nextAttempt - 1
	if idx < len(CleanupBackoff) {
		nextRetry = time.Now().Add(CleanupBackoff[idx])
	} else {
		nextRetry = time.Now().Add(CleanupBackoff[len(CleanupBackoff)-1])
	}
	status := "pending"
	if nextAttempt >= t.MaxAttempts {
		status = "exhausted"
	}
	w.db.Model(&CleanupTask{}).Where("id = ?", t.ID).Updates(map[string]any{
		"status":        status,
		"error":         truncateErr(err.Error()),
		"attempts":      nextAttempt,
		"next_retry_at": nextRetry,
	})
	log.Printf("[cleanup] task %s retry failed (kind=%s attempts=%d/%d): %v",
		t.ID, t.Kind, nextAttempt, t.MaxAttempts, err)
}

func (w *CleanupWorker) execute(ctx context.Context, t CleanupTask) error {
	switch t.Kind {
	case "milvus":
		if w.store == nil {
			return fmt.Errorf("vector store not configured")
		}
		return w.store.DeleteByDoc(ctx, t.KbID, t.DocID)
	case "minio":
		if t.ObjectKey == "" {
			return fmt.Errorf("empty object key")
		}
		return w.minio.RemoveObject(ctx, w.bucket, t.ObjectKey, minio.RemoveObjectOptions{})
	default:
		return fmt.Errorf("unknown cleanup kind: %s", t.Kind)
	}
}

func (w *CleanupWorker) markExhausted(t CleanupTask) {
	w.db.Model(&CleanupTask{}).Where("id = ?", t.ID).Updates(map[string]any{
		"status":        "exhausted",
		"next_retry_at": nil,
	})
	log.Printf("[cleanup] task %s exhausted (kind=%s doc=%s) — needs manual intervention",
		t.ID, t.Kind, t.DocID)
}

func truncateErr(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
