package server

import (
	"fmt"
	"log"
	"strings"

	"ollmo/ollmo/internal/agent"
	"ollmo/ollmo/internal/annotation"
	"ollmo/ollmo/internal/apikey"
	"ollmo/ollmo/internal/audit"
	"ollmo/ollmo/internal/bill"
	"ollmo/ollmo/internal/chat"
	"ollmo/ollmo/internal/config"
	"ollmo/ollmo/internal/doc"
	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/execution"
	"ollmo/ollmo/internal/graph"
	"ollmo/ollmo/internal/invitation"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/memory"
	"ollmo/ollmo/internal/pipeline"
	"ollmo/ollmo/internal/provider"
	"ollmo/ollmo/internal/rerank"
	"ollmo/ollmo/internal/site"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/internal/user"
	"ollmo/ollmo/pkg/db"
)

// RunMigrate applies GORM AutoMigrate for all domain models. Tables are kept
// independent; relationships are enforced at the service layer.
func RunMigrate(cfg *config.Config) error {
	gormDB, err := db.NewMySQL(cfg.MySQL)
	if err != nil {
		return err
	}

	if err := gormDB.AutoMigrate(
		&tenant.Tenant{},
		&tenant.TenantMember{},
		&user.User{},
		&kb.KnowledgeBase{},
		&doc.Document{},
		&doc.Chunk{},
		&annotation.Annotation{},
		&llm.LLMModel{},
		&embedding.EmbeddingModel{},
		&rerank.RerankModel{},
		&provider.Provider{},
		&chat.Conversation{},
		&chat.Message{},
		&pipeline.Pipeline{},
		&graph.Entity{},
		&graph.Relation{},
		&agent.Agent{},
		&execution.Execution{},
		&memory.Memory{},
		&doc.CleanupTask{},
		&apikey.APIKey{},
		&invitation.Invitation{},
		&audit.AuditLog{},
		&site.Setting{},
		&bill.Record{},
	); err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	// FULLTEXT index on chunks.content with the ngram parser so MySQL BM25
	// search works for CJK text as well as Latin. GORM AutoMigrate cannot
	// create FULLTEXT indexes, so this is handled explicitly. Idempotent:
	// skip if the index already exists from a previous run.
	var idxExists int64
	gormDB.Raw(`SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'chunks'
		AND index_name = 'idx_chunks_content_ft'`).Scan(&idxExists)
	if idxExists == 0 {
		if err := gormDB.Exec("CREATE FULLTEXT INDEX idx_chunks_content_ft ON chunks(content) WITH PARSER ngram").Error; err != nil {
			if !strings.Contains(err.Error(), "Duplicate") {
				return fmt.Errorf("create fulltext index: %w", err)
			}
		}
	}

	// Group pre-existing model rows under provider cards so the upgraded
	// settings page shows them. Idempotent: already-bound rows are skipped.
	if err := provider.Migrate(gormDB); err != nil {
		return fmt.Errorf("migrate providers: %w", err)
	}
	if err := provider.Backfill(gormDB); err != nil {
		return fmt.Errorf("backfill providers: %w", err)
	}

	log.Println("[migrate] schema applied: tenants, tenant_members, users, knowledge_bases, documents, chunks, llm_models, embedding_models, rerank_models, model_providers, conversations, messages, pipelines, graph_entities, graph_relations, agents, memories")
	log.Println("[migrate] fulltext index idx_chunks_content_ft (ngram parser) ensured on chunks.content")

	return nil
}
