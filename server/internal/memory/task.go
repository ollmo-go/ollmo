package memory

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

// TaskSummarizeConversation triggers an async conversation summary. It is
// enqueued by the chat service when a conversation crosses the auto-memory
// message threshold, and handled by Worker.HandleSummarize.
const TaskSummarizeConversation = "memory:summarize"

// SummarizePayload carries only IDs; the worker re-reads the conversation
// and its messages from MySQL so the task stays valid even if the payload
// was enqueued a while ago.
type SummarizePayload struct {
	TenantID       string `json:"tenant_id"`
	ConversationID string `json:"conversation_id"`
}

func NewSummarizeTask(p SummarizePayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	// TaskID deduplicates: if the same conversation is already queued or
	// processing, asynq rejects the duplicate instead of stacking summaries.
	return asynq.NewTask(TaskSummarizeConversation, b, asynq.TaskID("summarize-"+p.ConversationID)), nil
}

func DecodeSummarizePayload(t *asynq.Task) (SummarizePayload, error) {
	var p SummarizePayload
	err := json.Unmarshal(t.Payload(), &p)
	return p, err
}
