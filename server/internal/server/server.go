package server

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/hibiken/asynq"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"ollmo/ollmo/internal/chat"
	"ollmo/ollmo/internal/config"
	"ollmo/ollmo/internal/doc"
	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/graph"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/memory"
	"ollmo/ollmo/internal/pipeline"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/crypto"
	"ollmo/ollmo/pkg/db"
	"ollmo/ollmo/pkg/vector"
)

// Deps bundles initialized clients for the application lifetime. Both API
// and Worker share the same Deps so task handlers can use the same repos.
type Deps struct {
	cfg    *config.Config
	DB     *gorm.DB
	Redis  *redis.Client
	MinIO  *minio.Client
	Milvus client.Client
	Asynq  *asynq.Client
	MinerU *clients.MinerUClient
	Vector *vector.Store
	LLM    *clients.LLMClient
}

// RunAPI starts the Fiber HTTP server with all routes mounted.
func RunAPI(cfg *config.Config) error {
	deps, err := initDeps(cfg)
	if err != nil {
		return err
	}
	defer deps.Milvus.Close()

	app := fiber.New(fiber.Config{
		AppName:      "ollmo-api",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		BodyLimit:    50 * 1024 * 1024, // 50MB for document uploads
	})
	app.Use(recover.New())
	// Structured request logging: plain text in development for readability,
	// one JSON object per line in production so log shippers can parse it.
	logFmt := `${time} ${status} ${method} ${path} ${latency}` + "\n"
	if cfg.Server.Env == "production" {
		logFmt = `{"time":"${time}","level":"info","status":${status},"method":"${method}","path":"${path}","latency":"${latency}","ip":"${ip}","error":"${error}"}` + "\n"
	}
	app.Use(logger.New(logger.Config{Format: logFmt}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.Server.CORSOrigins,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
		AllowCredentials: true,
	}))

	// Rate limit: 100 requests per minute per IP. Protects login/LLM
	// endpoints from brute-force and abuse.
	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 60 * time.Second,
	}))

	registerRoutes(app, deps)

	// Graceful shutdown: wait for SIGINT/SIGTERM, then drain in-flight
	// requests for up to 30s before forcing the server to stop.
	go func() {
		log.Printf("[api] listening on :%s", cfg.Server.Port)
		if err := app.Listen(":" + cfg.Server.Port); err != nil {
			log.Printf("[api] server stopped: %v", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[api] shutting down (grace period 30s)...")
	if err := app.ShutdownWithTimeout(30 * time.Second); err != nil {
		log.Printf("[api] forced shutdown: %v", err)
	}
	return nil
}

// RunWorker starts the Asynq worker process. Task handlers will be registered
// by domain packages as they are implemented.
func RunWorker(cfg *config.Config) error {
	deps, err := initDeps(cfg)
	if err != nil {
		return err
	}
	defer deps.Milvus.Close()

	// Wire worker handlers. Same repos/clients as the API server; the only
	// difference is that the worker reads from Redis instead of HTTP.
	docRepo := doc.NewRepo(deps.DB)
	kbRepo := kb.NewRepo(deps.DB)
	embedRepo := embedding.NewRepo(deps.DB, crypto.FromPassphrase(deps.cfg.Auth.EncryptionKey))
	embedResolver := embedding.NewResolver(embedRepo, nil)
	// The worker's doc service publishes status events through Redis so the
	// API process (which owns the SSE connections) can relay them.
	docSvc := doc.NewService(docRepo, kbRepo, deps.MinIO, deps.cfg.MinIO.Bucket, deps.Asynq, deps.Vector, embedResolver).
		WithEventBus(doc.NewEventBus().WithRedis(deps.Redis))
	pipelineRepo := pipeline.NewRepo(deps.DB)
	pipelineSvc := pipeline.NewService(pipelineRepo, func(tenantID, kbID string) (pipeline.KBDefaults, error) {
		k, err := kbRepo.FindByID(tenantID, kbID)
		if err != nil {
			return pipeline.KBDefaults{}, err
		}
		// Resolve the pinned embedding model id to a name for the canvas
		// display (best-effort; empty when the model was deleted).
		embName := ""
		if emb, err := embedRepo.FindByID(tenantID, k.EmbeddingModelID); err == nil {
			embName = emb.Model
		}
		return pipeline.KBDefaults{
			KbID: k.ID, TenantID: k.TenantID,
			EmbeddingModel: embName, ChunkStrategy: k.ChunkStrategy,
			ChunkSize: k.ChunkSize, ChunkOverlap: k.ChunkOverlap,
		}, nil
	})
	llmRepo := llm.NewRepo(deps.DB, crypto.FromPassphrase(deps.cfg.Auth.EncryptionKey))
	graphSvc := graph.NewService(graph.NewRepo(deps.DB), deps.LLM)
	docWorker := doc.NewWorker(
		docSvc, kbRepo, deps.MinIO, deps.cfg.MinIO.Bucket, deps.MinerU,
		embedResolver, deps.Vector,
		pipelineSvc.ExtractConfig,
		graphSvc, llmRepo,
	).WithQuotaChecker(&quotaAdapter{repo: tenant.NewRepo(deps.DB)})

	// Two independent worker pools share one mux. The pipeline pool serves
	// the "parse" queue (doc:parse/embed/extract, including 30-minute MinerU
	// jobs) with its own concurrency; the aux pool serves "default" (short
	// interactive tasks such as conversation summaries). Separate pools mean
	// a batch of long parses can never occupy the slots that summaries need,
	// and vice versa.
	redisOpt := asynq.RedisClientOpt{Addr: cfg.Redis.Addr(), Password: cfg.Redis.Password}
	pipelineSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.Worker.PipelineConcurrency,
		Queues:      map[string]int{doc.QueuePipeline: 1},
	})
	auxSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.Worker.AuxConcurrency,
		Queues:      map[string]int{doc.QueueDefault: 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(doc.TaskParseDocument, docWorker.HandleParse)
	mux.HandleFunc(doc.TaskEmbedDocument, docWorker.HandleEmbed)
	mux.HandleFunc(doc.TaskExtractDocument, docWorker.HandleExtract)

	// Memory: async conversation summarization (auto memory).
	memSvc := memory.NewService(memory.NewRepo(deps.DB), chat.NewRepo(deps.DB), llmRepo, deps.LLM)
	memWorker := memory.NewWorker(memSvc)
	mux.HandleFunc(memory.TaskSummarizeConversation, memWorker.HandleSummarize)

	// Start the cleanup sweep alongside the Asynq worker so failed
	// Milvus/MinIO deletions are retried without manual intervention.
	cleanupWorker := doc.NewCleanupWorker(deps.DB, deps.Vector, deps.MinIO, deps.cfg.MinIO.Bucket)
	go cleanupWorker.Start(context.Background())

	log.Printf("[worker] starting: queue %q concurrency=%d (doc pipeline), queue %q concurrency=%d (aux); handlers: %s, %s, %s, %s",
		doc.QueuePipeline, cfg.Worker.PipelineConcurrency,
		doc.QueueDefault, cfg.Worker.AuxConcurrency,
		doc.TaskParseDocument, doc.TaskEmbedDocument, doc.TaskExtractDocument, memory.TaskSummarizeConversation)

	// Both pools watch for SIGTERM/SIGINT and shut down gracefully. The aux
	// pool runs in a goroutine; RunWorker blocks on the pipeline pool.
	go func() {
		if err := auxSrv.Run(mux); err != nil {
			log.Printf("[worker] aux pool exited: %v", err)
		}
	}()
	return pipelineSrv.Run(mux)
}

func initDeps(cfg *config.Config) (*Deps, error) {
	gormDB, err := db.NewMySQL(cfg.MySQL)
	if err != nil {
		return nil, err
	}
	rdb := db.NewRedis(cfg.Redis)
	mc, err := db.NewMinIO(cfg.MinIO)
	if err != nil {
		return nil, err
	}
	mv, err := db.NewMilvus(cfg.Milvus)
	if err != nil {
		return nil, err
	}
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
	})
	minerUClient := clients.NewMinerU(cfg.MinerU.Endpoint)
	store := vector.NewStore(mv)
	llmClient := clients.NewLLM()
	return &Deps{
		cfg: cfg, DB: gormDB, Redis: rdb, MinIO: mc, Milvus: mv,
		Asynq:  asynqClient,
		MinerU: minerUClient,
		Vector: store,
		LLM:    llmClient,
	}, nil
}
