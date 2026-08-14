package memory

import (
	"context"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
)

// Worker is the Asynq handler for memory tasks. It runs in the worker
// process (separate from the API) so summarization LLM calls never block
// user-facing requests.
type Worker struct {
	svc *Service
}

func NewWorker(svc *Service) *Worker { return &Worker{svc: svc} }

// HandleSummarize processes a memory:summarize task: regenerate the
// conversation summary and upsert the memory row.
func (w *Worker) HandleSummarize(ctx context.Context, t *asynq.Task) error {
	p, err := DecodeSummarizePayload(t)
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if _, err := w.svc.SummarizeByID(ctx, p.TenantID, p.ConversationID); err != nil {
		// Returning the error lets asynq retry with backoff. Transient LLM
		// failures resolve on retry; permanent ones (no provider) exhaust
		// retries and land in the dead-letter queue.
		return fmt.Errorf("summarize conv=%s: %w", p.ConversationID, err)
	}
	log.Printf("[memory] auto-summary saved tenant=%s conv=%s", p.TenantID, p.ConversationID)
	return nil
}
