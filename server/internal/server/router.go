package server

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/agent"
	"ollmo/ollmo/internal/analytics"
	"ollmo/ollmo/internal/annotation"
	"ollmo/ollmo/internal/apikey"
	"ollmo/ollmo/internal/audit"
	"ollmo/ollmo/internal/auth"
	"ollmo/ollmo/internal/backup"
	"ollmo/ollmo/internal/bill"
	"ollmo/ollmo/internal/chat"
	"ollmo/ollmo/internal/doc"
	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/execution"
	"ollmo/ollmo/internal/graph"
	"ollmo/ollmo/internal/install"
	"ollmo/ollmo/internal/invitation"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/memory"
	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/internal/pipeline"
	"ollmo/ollmo/internal/provider"
	"ollmo/ollmo/internal/rerank"
	"ollmo/ollmo/internal/search"
	"ollmo/ollmo/internal/site"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/internal/user"
	"ollmo/ollmo/pkg/crypto"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

// registerRoutes wires all HTTP routes onto the Fiber app. Domain handlers
// are constructed here; cross-domain dependencies are passed explicitly.
func registerRoutes(app *fiber.App, deps *Deps, bgCtx context.Context) {
	// /health is a cheap liveness probe: the process is up and serving.
	app.Get("/health", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{"status": "ok", "service": "ollmo-api"})
	})

	// /ready is a readiness probe: it pings every infrastructure dependency
	// and returns 503 when any is unreachable. Use it for Docker/K8s
	// readiness checks so traffic is only routed when the API can actually
	// serve requests. Each check has its own short timeout so a hung dep
	// cannot stall the probe.
	app.Get("/ready", func(c *fiber.Ctx) error {
		checks := fiber.Map{}
		healthy := true
		ping := func(name string, check func(ctx context.Context) error) {
			ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
			defer cancel()
			if err := check(ctx); err != nil {
				checks[name] = fiber.Map{"status": "down", "error": err.Error()}
				healthy = false
				return
			}
			checks[name] = fiber.Map{"status": "ok"}
		}
		ping("mysql", func(ctx context.Context) error {
			sqlDB, err := deps.DB.DB()
			if err != nil {
				return err
			}
			return sqlDB.PingContext(ctx)
		})
		ping("redis", func(ctx context.Context) error {
			return deps.Redis.Ping(ctx).Err()
		})
		ping("minio", func(ctx context.Context) error {
			_, err := deps.MinIO.BucketExists(ctx, deps.cfg.MinIO.Bucket)
			return err
		})
		ping("milvus", func(ctx context.Context) error {
			_, err := deps.Milvus.ListCollections(ctx)
			return err
		})
		code := 200
		status := "ok"
		if !healthy {
			code = 503
			status = "degraded"
		}
		return c.Status(code).JSON(fiber.Map{"status": status, "checks": checks})
	})

	userRepo := user.NewRepo(deps.DB)
	tenantRepo := tenant.NewRepo(deps.DB)
	siteSvc := site.NewService(site.NewRepo(deps.DB))
	if err := siteSvc.Load(); err != nil {
		log.Printf("[api] warning: failed to load site settings: %v", err)
	}
	siteHandler := site.NewHandler(siteSvc)
	tokenRevoker := middleware.NewRedisTokenRevoker(deps.Redis)
	loginAttempts := auth.NewRedisLoginAttempts(deps.Redis)
	authSvc := auth.NewService(userRepo, tenantRepo, deps.cfg.Auth.JWTSecret, deps.cfg.Auth.JWTExpireHours).
		WithQuotas(&quotaConfigAdapter{cfg: deps.cfg.Quota}).
		WithRevoker(tokenRevoker).
		WithLoginAttempts(loginAttempts).
		WithRegistrationChecker(siteSvc)
	authHandler := auth.NewHandler(authSvc, deps.cfg.Auth.JWTExpireHours)
	userHandler := user.NewHandler(userRepo, tenantRepo)

	kbRepo := kb.NewRepo(deps.DB)
	embedRepo := embedding.NewRepo(deps.DB, crypto.FromPassphrase(deps.cfg.Auth.EncryptionKey))
	kbSvc := kb.NewService(kbRepo, deps.Vector).
		WithEmbeddingDefault(func(tenantID string) (string, error) {
			p, err := embedRepo.FindDefault(tenantID)
			if err != nil {
				return "", err
			}
			return p.ID, nil
		}).
		WithEmbeddingResolver(&embeddingResolverAdapter{repo: embedRepo})
	kbHandler := kb.NewHandler(kbSvc)

	// Quota checker: adapter from tenant.Repo + Redis to doc.QuotaChecker
	// and chat.MessageQuotaChecker. Keeps doc/chat packages decoupled from
	// tenant and Redis at import time.
	quotaChecker := &quotaAdapter{repo: tenantRepo, rdb: deps.Redis}
	// Per-user daily message usage for the member list (batched MGET).
	userHandler.WithMessageUsage(quotaChecker.UserMessageUsageBatch)

	// Pipeline: per-KB ingestion canvas config.
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
			KbID:           k.ID,
			TenantID:       k.TenantID,
			EmbeddingModel: embName,
			ChunkStrategy:  k.ChunkStrategy,
			ChunkSize:      k.ChunkSize,
			ChunkOverlap:   k.ChunkOverlap,
		}, nil
	})
	pipelineHandler := pipeline.NewHandler(pipelineSvc)

	// Agent: per-KB Q&A flow canvas.
	agentRepo := agent.NewRepo(deps.DB)
	agentSvc := agent.NewService(agentRepo)
	agentHandler := agent.NewHandler(agentSvc)

	// Documents (nested under KB). Created early so the install wizard can
	// seed a demo document; the route group below reuses this instance.
	docRepo := doc.NewRepo(deps.DB)
	embedResolver := embedding.NewResolver(embedRepo, nil)
	// Redis relay: the worker publishes doc events to Redis; this goroutine
	// feeds them to local SSE subscribers.
	docEventBus := doc.NewEventBus().WithRedis(deps.Redis)
	go docEventBus.RelayRedis(context.Background())
	docSvc := doc.NewService(docRepo, kbRepo, deps.MinIO, deps.cfg.MinIO.Bucket, deps.Asynq, deps.Vector, embedResolver).
		WithQuotaChecker(quotaChecker).
		WithEventBus(docEventBus)
	kbSvc.WithReembedder(docSvc)
	docHandler := doc.NewHandler(docSvc)

	// Invitation: user invitation flow.
	invRepo := invitation.NewRepo(deps.DB)
	invSvc := invitation.NewService(invRepo, userRepo)
	invHandler := invitation.NewHandler(invSvc)

	api := app.Group("/api/v1")

	// Install routes (public, only work when system is uninitialized).
	// Unified provider service so install seeds one provider card with all model kinds.
	installCrypt := crypto.FromPassphrase(deps.cfg.Auth.EncryptionKey)
	installSvc := install.NewService(
		deps.DB, userRepo, tenantRepo,
		provider.NewService(
			provider.NewRepo(deps.DB, installCrypt),
			provider.NewLLMStore(llm.NewRepo(deps.DB, installCrypt)),
			provider.NewEmbeddingStore(embedding.NewRepo(deps.DB, installCrypt)),
			provider.NewRerankStore(rerank.NewRepo(deps.DB, installCrypt)),
		),
		kbSvc, agentSvc, docSvc,
		deps.cfg.Auth.JWTSecret, deps.cfg.Auth.JWTExpireHours,
	)
	installHandler := install.NewHandler(installSvc)
	api.Get("/install/status", installHandler.Status)
	api.Get("/install/defaults", installHandler.Defaults)
	api.Post("/install", installHandler.Install)

	// Public site settings (site name, language, registration flag).
	api.Get("/site/settings", siteHandler.GetPublic)

	// Public auth routes (no auth required). Registration respects the
	// allow_registration site setting.
	api.Post("/auth/register", func(c *fiber.Ctx) error {
		if siteSvc.Get(site.KeyAllowRegistration) != "true" {
			return response.Fail(c, errs.Forbidden("registration is disabled"))
		}
		return authHandler.Register(c)
	})
	api.Post("/auth/login", authHandler.Login)

	// Public invitation routes (no auth required, token-based).
	api.Get("/invitations/:token", invHandler.Peek)
	api.Post("/invitations/accept", invHandler.Accept)

	// Authenticated routes.
	protected := api.Group("/", middleware.Auth(deps.cfg.Auth.JWTSecret, tokenRevoker))

	// Per-tenant rate limiting: 1000 requests/minute per tenant, on top of
	// the global per-IP limiter. Keeps a single noisy tenant from saturating
	// the API while leaving headroom for normal interactive usage.
	protected.Use(middleware.TenantRateLimit(deps.Redis, 1000, time.Minute))

	// Audit logging for write operations (POST/PUT/DELETE). Read-only
	// requests are not audited to keep the volume manageable.
	auditSvc := audit.NewService(audit.NewRepo(deps.DB))
	protected.Use(audit.Middleware(auditSvc))

	protected.Get("/auth/me", authHandler.Me)
	protected.Post("/auth/logout", authHandler.Logout)

	// Current user profile (self-service, not admin-only).
	protected.Get("/user/profile", userHandler.GetProfile)
	protected.Put("/user/profile", userHandler.UpdateProfile)
	protected.Put("/user/password", userHandler.ChangePassword)

	// Tenant quota (team-level data; admin only).
	protected.Get("/tenant/quota", middleware.AdminOnly(), func(c *fiber.Ctx) error {
		tid := middleware.TenantID(c)
		t, err := tenantRepo.FindByID(tid)
		if err != nil {
			return response.Fail(c, errs.NotFound("tenant not found"))
		}
		docCount, _ := tenantRepo.CountDocs(tid)
		chunkCount, _ := tenantRepo.CountChunks(tid)
		msgUsed, _, _ := quotaChecker.TenantMessageUsage(tid)
		memberCount, _ := userRepo.CountByTenant(tid)
		const warnRatio = 0.8
		return response.OK(c, fiber.Map{
			"plan":         t.Plan,
			"name":         t.Name,
			"member_count": memberCount,
			"doc": fiber.Map{
				"used": docCount, "limit": t.DocQuota,
				"warning": t.DocQuota > 0 && docCount >= int64(float64(t.DocQuota)*warnRatio),
			},
			"vector": fiber.Map{
				"used": chunkCount, "limit": t.VectorQuota,
				"warning": t.VectorQuota > 0 && chunkCount >= int64(float64(t.VectorQuota)*warnRatio),
			},
			"message": fiber.Map{
				"used": msgUsed, "limit": t.MessageQuota,
				"warning": t.MessageQuota > 0 && msgUsed >= int(float64(t.MessageQuota)*warnRatio),
			},
			"user_message_limit": t.UserMessageQuota,
		})
	})

	// Tenant user management (admin only).
	adminGrp := protected.Group("/tenant", middleware.AdminOnly())
	adminGrp.Put("/", func(c *fiber.Ctx) error {
		var in struct {
			Name string `json:"name"`
		}
		if err := c.BodyParser(&in); err != nil {
			return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
		}
		if in.Name == "" {
			return response.Fail(c, errs.BadRequest("name is required"))
		}
		if err := tenantRepo.UpdateName(middleware.TenantID(c), in.Name); err != nil {
			return response.Fail(c, errs.Wrap(errs.CodeInternal, "update tenant name", err))
		}
		return response.OK(c, fiber.Map{"status": "updated"})
	})
	adminGrp.Get("/users", userHandler.ListUsers)
	adminGrp.Put("/users/:userId/role", userHandler.UpdateRole)
	adminGrp.Put("/users/:userId/status", userHandler.UpdateStatus)
	adminGrp.Post("/users/:userId/reset-password", userHandler.ResetPassword)

	// Invitation management (admin only).
	adminGrp.Get("/invitations", invHandler.List)
	adminGrp.Post("/invitations", invHandler.Create)
	adminGrp.Delete("/invitations/:id", invHandler.Cancel)

	// System settings (super admin only).
	siteGrp := protected.Group("/site", middleware.SuperAdminOnly())
	siteGrp.Get("/settings/all", siteHandler.GetAll)
	siteGrp.Put("/settings", siteHandler.Update)

	// User management (super admin only): list all users and toggle super admin.
	sysAdminGrp := protected.Group("/admin", middleware.SuperAdminOnly())
	sysAdminGrp.Get("/users", userHandler.ListAllUsers)
	sysAdminGrp.Put("/users/:userId/super-admin", userHandler.UpdateSuperAdmin)

	// Tenant management (super admin only): list all tenants and adjust quotas.
	sysAdminGrp.Get("/tenants", func(c *fiber.Ctx) error {
		ts, err := tenantRepo.ListAll()
		if err != nil {
			return response.Fail(c, errs.Wrap(errs.CodeInternal, "list tenants", err))
		}
		// Batch counts to avoid N+1 queries (one per tenant).
		memberCounts, _ := userRepo.CountByTenants()
		docCounts, _ := tenantRepo.CountDocsByTenant()
		ids := make([]string, len(ts))
		for i, t := range ts {
			ids[i] = t.ID
		}
		msgUsed := quotaChecker.TenantMessageUsageBatch(ids)
		type row struct {
			tenant.Tenant
			MemberCount int64 `json:"member_count"`
			DocUsed     int64 `json:"doc_used"`
			MsgUsed     int64 `json:"msg_used"`
		}
		rows := make([]row, 0, len(ts))
		for _, t := range ts {
			rows = append(rows, row{
				Tenant:      t,
				MemberCount: memberCounts[t.ID],
				DocUsed:     docCounts[t.ID],
				MsgUsed:     int64(msgUsed[t.ID]),
			})
		}
		return response.OK(c, rows)
	})
	sysAdminGrp.Put("/tenants/:id/quota", func(c *fiber.Ctx) error {
		var in struct {
			DocQuota         int `json:"doc_quota"`
			VectorQuota      int `json:"vector_quota"`
			MessageQuota     int `json:"message_quota"`
			UserMessageQuota int `json:"user_message_quota"`
		}
		if err := c.BodyParser(&in); err != nil {
			return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
		}
		// -1 means unlimited; reject other negative values.
		for _, v := range []int{in.DocQuota, in.VectorQuota, in.MessageQuota, in.UserMessageQuota} {
			if v < -1 {
				return response.Fail(c, errs.BadRequest("quota must be >= -1"))
			}
		}
		if err := tenantRepo.UpdateQuotas(c.Params("id"), in.DocQuota, in.VectorQuota, in.MessageQuota, in.UserMessageQuota); err != nil {
			return response.Fail(c, errs.Wrap(errs.CodeInternal, "update quotas", err))
		}
		return response.OK(c, fiber.Map{"status": "updated"})
	})
	sysAdminGrp.Put("/tenants/:id/plan", func(c *fiber.Ctx) error {
		var in struct {
			Plan string `json:"plan"`
		}
		if err := c.BodyParser(&in); err != nil {
			return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
		}
		plan := strings.ToLower(strings.TrimSpace(in.Plan))
		switch plan {
		case "free", "pro", "enterprise":
		default:
			return response.Fail(c, errs.BadRequest("plan must be one of: free, pro, enterprise"))
		}
		q := deps.cfg.Quota.QuotasFor(plan)
		if err := tenantRepo.UpdatePlan(c.Params("id"), plan, q.DocQuota, q.VectorQuota, q.MessageQuota, q.UserMessageQuota); err != nil {
			return response.Fail(c, errs.Wrap(errs.CodeInternal, "update plan", err))
		}
		return response.OK(c, fiber.Map{"status": "updated"})
	})

	// Knowledge bases.
	kbRead := kbAccess(kbRepo, false)
	kbWrite := kbAccess(kbRepo, true)

	kbGrp := protected.Group("/knowledge-bases")
	// Creating KBs is a management action; members only chat with existing KBs.
	kbGrp.Post("/", middleware.AdminOnly(), func(c *fiber.Ctx) error {
		var in kb.CreateInput
		if err := c.BodyParser(&in); err != nil {
			return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
		}
		k, err := kbSvc.Create(c.Context(), middleware.TenantID(c), middleware.UserID(c), in)
		if err != nil {
			return response.Fail(c, err)
		}
		return response.Created(c, k)
	})
	kbGrp.Get("/", kbHandler.List)
	kbGrp.Get("/:id", kbRead, kbHandler.Get)
	kbGrp.Put("/:id", kbWrite, kbHandler.Update)
	kbGrp.Delete("/:id", kbWrite, kbHandler.Delete)
	kbGrp.Put("/:id/visibility", kbWrite, kbHandler.SetVisibility)

	// Ingestion pipeline canvas.
	kbGrp.Get("/:kbId/pipeline", kbRead, pipelineHandler.Get)
	kbGrp.Put("/:kbId/pipeline", kbWrite, pipelineHandler.Save)

	// Agent canvas.
	kbGrp.Get("/:kbId/agent", kbRead, agentHandler.Get)
	kbGrp.Put("/:kbId/agent", kbWrite, agentHandler.Save)

	// Documents (nested under KB). The service instance is created above so
	// the install wizard can reuse it for the demo document.
	docGrp := kbGrp.Group("/:kbId/documents")
	docGrp.Post("/", kbWrite, docHandler.Upload)
	docGrp.Get("/", kbRead, docHandler.List)
	docGrp.Post("/preview", kbRead, docHandler.PreviewChunks)
	docGrp.Post("/import-url", kbWrite, docHandler.ImportURL)
	docGrp.Post("/batch-delete", kbWrite, docHandler.BatchDelete)
	docGrp.Post("/batch-reparse", kbWrite, docHandler.BatchReparse)
	docGrp.Get("/:id", kbRead, docHandler.Get)
	docGrp.Get("/:id/content", kbRead, docHandler.Content)
	docGrp.Get("/:id/original", kbRead, docHandler.Original)
	docGrp.Get("/:id/chunks", kbRead, docHandler.ListChunks)
	docGrp.Put("/:id/chunks/:chunkId", kbWrite, docHandler.UpdateChunk)
	docGrp.Delete("/:id/chunks/:chunkId", kbWrite, docHandler.DeleteChunk)
	docGrp.Delete("/:id", kbWrite, docHandler.Delete)
	docGrp.Post("/:id/reparse", kbWrite, docHandler.Reparse)
	docGrp.Put("/:id/enabled", kbWrite, docHandler.SetEnabled)
	docGrp.Put("/:id/metadata", kbWrite, docHandler.UpdateMetadata)

	// SSE stream for real-time document status updates.
	protected.Get("/documents/events", doc.SSEHandler(docEventBus))

	// Annotation replies (curated Q&A matched by embedding similarity).
	annSvc := annotation.NewService(annotation.NewRepo(deps.DB), kbRepo, deps.Vector, embedResolver)
	kbSvc.WithAnnSyncer(annSvc)
	annHandler := annotation.NewHandler(annSvc)
	annGrp := kbGrp.Group("/:kbId/annotations")
	annGrp.Get("/", kbRead, annHandler.List)
	annGrp.Post("/", kbWrite, annHandler.Create)
	annGrp.Put("/:id", kbWrite, annHandler.Update)
	annGrp.Delete("/:id", kbWrite, annHandler.Delete)

	// LLM models.
	llmRepo := llm.NewRepo(deps.DB, crypto.FromPassphrase(deps.cfg.Auth.EncryptionKey))
	llmSvc := llm.NewService(llmRepo, deps.LLM)
	llmHandler := llm.NewHandler(llmSvc)
	llmGrp := protected.Group("/llm-models")
	llmGrp.Get("/", llmHandler.List)
	llmGrp.Get("/:id", llmHandler.Get)
	llmGrp.Post("/:id/test", middleware.AdminOnly(), llmHandler.Test)
	llmGrp.Post("/", middleware.AdminOnly(), llmHandler.Create)
	llmGrp.Put("/:id", middleware.AdminOnly(), llmHandler.Update)
	llmGrp.Delete("/:id", middleware.AdminOnly(), llmHandler.Delete)

	// Embedding models.
	embedSvc := embedding.NewService(embedRepo)
	embedHandler := embedding.NewHandler(embedSvc)
	embedGrp := protected.Group("/embedding-models")
	embedGrp.Get("/", embedHandler.List)
	embedGrp.Get("/:id", embedHandler.Get)
	embedGrp.Post("/:id/test", middleware.AdminOnly(), embedHandler.Test)
	embedGrp.Post("/", middleware.AdminOnly(), embedHandler.Create)
	embedGrp.Put("/:id", middleware.AdminOnly(), embedHandler.Update)
	embedGrp.Delete("/:id", middleware.AdminOnly(), embedHandler.Delete)

	// Rerank models.
	rerankRepo := rerank.NewRepo(deps.DB, crypto.FromPassphrase(deps.cfg.Auth.EncryptionKey))
	rerankResolver := rerank.NewResolver(rerankRepo, nil, "")
	rerankSvc := rerank.NewService(rerankRepo)
	rerankHandler := rerank.NewHandler(rerankSvc)
	rerankGrp := protected.Group("/rerank-models")
	rerankGrp.Get("/", rerankHandler.List)
	rerankGrp.Get("/:id", rerankHandler.Get)
	rerankGrp.Post("/:id/test", middleware.AdminOnly(), rerankHandler.Test)
	rerankGrp.Post("/", middleware.AdminOnly(), rerankHandler.Create)
	rerankGrp.Put("/:id", middleware.AdminOnly(), rerankHandler.Update)
	rerankGrp.Delete("/:id", middleware.AdminOnly(), rerankHandler.Delete)

	// Unified providers management (方案B: 统一provider实体)
	crypt := crypto.FromPassphrase(deps.cfg.Auth.EncryptionKey)
	providerSvc := provider.NewService(
		provider.NewRepo(deps.DB, crypt),
		provider.NewLLMStore(llmRepo),
		provider.NewEmbeddingStore(embedRepo),
		provider.NewRerankStore(rerankRepo),
	)
	providerHandler := provider.NewHandler(providerSvc)
	providerGrp := protected.Group("/providers", middleware.AdminOnly())
	providerGrp.Get("/", providerHandler.List)
	providerGrp.Get("/catalog", providerHandler.Catalog)
	providerGrp.Post("/", providerHandler.Create)
	providerGrp.Put("/:id", providerHandler.Update)
	providerGrp.Delete("/:id", providerHandler.Delete)
	providerGrp.Post("/:id/discover", providerHandler.Discover)
	providerGrp.Post("/probe", providerHandler.Probe)
	providerGrp.Post("/probe-model", providerHandler.ProbeModel)
	providerGrp.Post("/:id/models/:kind", providerHandler.AddModel)
	providerGrp.Put("/:id/models/:kind/:mid", providerHandler.UpdateModel)
	providerGrp.Delete("/:id/models/:kind/:mid", providerHandler.RemoveModel)

	// Search (retrieval).
	graphSvc := graph.NewService(graph.NewRepo(deps.DB), deps.LLM)
	graphHandler := graph.NewHandler(graphSvc)
	searchSvc := search.NewService(kbRepo, docRepo, embedResolver, deps.Vector, rerankResolver, graphSvc)
	searchHandler := search.NewHandler(searchSvc)
	protected.Post("/knowledge-bases/:kbId/search", kbRead, searchHandler.Search)
	protected.Post("/knowledge-bases/:kbId/search/debug", kbRead, searchHandler.Debug)

	// Knowledge graph entities (GraphRAG).
	kbGrp.Get("/:kbId/entities", kbRead, graphHandler.List)

	// Execution history: persisted agent-graph runs for replay. Traces carry
	// users' questions/answers, so only KB admins/owners see them.
	executionRepo := execution.NewRepo(deps.DB)
	executionHandler := execution.NewHandler(executionRepo, func(tenantID string, userIDs []string) map[string]string {
		m, err := userRepo.FindByIDs(userIDs)
		if err != nil {
			return nil
		}
		names := make(map[string]string, len(m))
		for id, u := range m {
			names[id] = u.Name
		}
		return names
	})
	kbGrp.Get("/:kbId/executions", kbWrite, executionHandler.List)
	kbGrp.Get("/:kbId/executions/by-message/:messageId", kbWrite, executionHandler.GetByMessage)
	protected.Get("/executions/:id", middleware.AdminOnly(), executionHandler.Get)

	// Chat: conversations and messages with SSE streaming.
	billSvc := bill.NewService(bill.NewRepo(deps.DB))
	memorySvc := memory.NewService(memory.NewRepo(deps.DB), chat.NewRepo(deps.DB), llmRepo, deps.LLM).
		WithAsynq(deps.Asynq)
	memoryHandler := memory.NewHandler(memorySvc)
	// Both closures hit GetDefinition, which caches the parsed definition per
	// (tenant, kb): one chat turn needs the same agent row twice (execution
	// config extraction + graph walk) and pays only one DB read + unmarshal.
	chatSvc := chat.NewService(chat.NewRepo(deps.DB), searchSvc, llmRepo, deps.LLM).
		WithBackgroundContext(bgCtx).
		WithAgentConfig(func(ctx context.Context, tenantID, kbID string) (*agent.ExecutionConfig, error) {
			_, def, err := agentSvc.GetDefinition(ctx, tenantID, kbID)
			if err != nil || def == nil {
				return nil, nil
			}
			cfg := agent.ExtractConfig(def)
			return &cfg, nil
		}).
		WithAgentDefinition(func(ctx context.Context, tenantID, kbID string) (*agent.Definition, error) {
			_, def, err := agentSvc.GetDefinition(ctx, tenantID, kbID)
			if err != nil || def == nil {
				return nil, nil
			}
			return def, nil
		}).
		WithMemoryContext(func(ctx context.Context, tenantID, userID, kbID string) (string, error) {
			return memorySvc.BuildContext(ctx, tenantID, userID, kbID, 5)
		}).
		WithAutoMemory(func(tenantID, convID string) {
			// Site-level kill switch: when auto memory is disabled, skip the
			// threshold check entirely (no DB count, no enqueue).
			if !siteSvc.AutoMemoryEnabled() {
				return
			}
			memorySvc.MaybeEnqueueAutoSummary(tenantID, convID)
		}).
		WithConversationDeleted(func(tenantID, convID string) {
			memorySvc.DeleteByConversation(tenantID, convID)
		}).
		WithMessageQuota(quotaChecker).
		WithAnnotations(annSvc).
		WithExecutionRepo(executionRepo).
		WithBill(billSvc)
	chatHandler := chat.NewHandler(chatSvc)
	protected.Post("/knowledge-bases/:kbId/conversations", kbRead, chatHandler.Create)
	protected.Get("/conversations", chatHandler.List)
	protected.Get("/conversations/:id", chatHandler.Get)
	protected.Put("/conversations/:id", chatHandler.Rename)
	protected.Put("/conversations/:id/pin", chatHandler.SetPinned)
	protected.Get("/conversations/:id/export", chatHandler.Export)
	protected.Get("/conversations/:id/messages", chatHandler.ListMessages)
	protected.Post("/conversations/:id/messages/:messageId/vote", chatHandler.VoteMessage)
	protected.Delete("/conversations/:id", chatHandler.Delete)
	protected.Post("/conversations/:id/messages/stream", chatHandler.Stream)
	protected.Get("/conversations/:id/stream", chatHandler.Subscribe)
	protected.Post("/knowledge-bases/:kbId/test-chat", kbWrite, chatHandler.TestChat)
	protected.Post("/knowledge-bases/:kbId/debug-node", kbWrite, chatHandler.DebugNode)
	protected.Get("/quota/messages", chatHandler.MessageQuota)

	// Memory: conversation summaries for cross-session context.
	protected.Post("/conversations/:id/summarize", memoryHandler.Summarize)
	kbGrp.Get("/:kbId/memories", kbRead, memoryHandler.List)
	kbGrp.Delete("/:kbId/memories/:id", kbWrite, memoryHandler.Delete)

	// API Key management (JWT-protected, admin only: keys grant programmatic
	// access to the tenant's KBs, so members cannot mint their own).
	apikeyRepo := apikey.NewRepo(deps.DB)
	apikeySvc := apikey.NewService(apikeyRepo)
	apikeyHandler := apikey.NewHandler(apikeySvc)
	keyGrp := protected.Group("/api-keys", middleware.AdminOnly())
	keyGrp.Post("/", apikeyHandler.Create)
	keyGrp.Get("/", apikeyHandler.List)
	keyGrp.Put("/:id/revoke", apikeyHandler.Revoke)
	keyGrp.Delete("/:id", apikeyHandler.Delete)

	// External API (API Key-protected). Same handlers as the JWT routes;
	// the API key middleware sets the same Locals keys, so handlers work
	// unchanged. Exposes the core RAG endpoints for programmatic access.
	external := api.Group("/external", apikey.Middleware(apikeyRepo))
	external.Get("/knowledge-bases", kbHandler.List)
	external.Post("/knowledge-bases", kbHandler.Create)
	external.Get("/knowledge-bases/:kbId", kbAccess(kbRepo, false), kbHandler.Get)
	external.Put("/knowledge-bases/:kbId", kbAccess(kbRepo, true), kbHandler.Update)
	external.Delete("/knowledge-bases/:kbId", kbAccess(kbRepo, true), kbHandler.Delete)
	external.Get("/knowledge-bases/:kbId/documents", kbAccess(kbRepo, false), docHandler.List)
	external.Post("/knowledge-bases/:kbId/documents", kbAccess(kbRepo, true), docHandler.Upload)
	external.Post("/knowledge-bases/:kbId/search", kbAccess(kbRepo, false), searchHandler.Search)
	external.Get("/conversations", chatHandler.List)
	external.Post("/conversations/:id/messages/stream", chatHandler.Stream)

	// Analytics dashboard (admin-only team stats).
	analyticsSvc := analytics.NewService(analytics.NewRepo(deps.DB))
	analyticsHandler := analytics.NewHandler(analyticsSvc)
	analyticsGrp := protected.Group("/analytics", middleware.AdminOnly())
	analyticsGrp.Get("/overview", analyticsHandler.Overview)
	analyticsGrp.Get("/documents", analyticsHandler.DocStats)
	analyticsGrp.Get("/usage", analyticsHandler.KBUsage)
	analyticsGrp.Get("/activity", analyticsHandler.RecentActivity)
	analyticsGrp.Get("/feedback", analyticsHandler.Feedback)

	// Usage & cost (bill rows written per LLM call). /mine is the personal
	// view used by the chat profile dialog (any authenticated user); the
	// tenant-wide aggregates are admin-only team-management data, so members
	// never see other users' consumption.
	billHandler := bill.NewHandler(billSvc)
	billGrp := protected.Group("/bills")
	billGrp.Get("/mine", billHandler.Mine)
	billGrp.Get("/overview", middleware.AdminOnly(), billHandler.Overview)
	billGrp.Get("/users", middleware.AdminOnly(), billHandler.Users)
	billGrp.Get("/models", middleware.AdminOnly(), billHandler.Models)
	billGrp.Get("/records", middleware.AdminOnly(), billHandler.Records)

	// Audit logging (admin only).
	auditHandler := audit.NewHandler(auditSvc)
	adminGrp.Get("/audit-logs", auditHandler.List)

	// Backup / restore (admin only).
	backupSvc := backup.NewService(deps.DB)
	backupHandler := backup.NewHandler(backupSvc)
	adminGrp.Get("/knowledge-bases/:kbId/export", backupHandler.Export)
	adminGrp.Post("/knowledge-bases/:kbId/import", backupHandler.Import)
}

// kbAccess returns middleware that checks the caller can access the KB
// identified by the :kbId (or :id) path param. Tenant admins and super
// admins bypass all KB-level checks. KB owners always pass. For team-visible
// KBs, any tenant member gets read access. When write is true, only admin or
// owner pass.
func kbAccess(kbRepo *kb.Repo, write bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if middleware.Role(c) == "admin" || middleware.IsSuperAdmin(c) {
			return c.Next()
		}

		kbID := c.Params("kbId")
		if kbID == "" {
			kbID = c.Params("id")
		}
		if kbID == "" {
			return c.Next()
		}

		tenantID := middleware.TenantID(c)
		userID := middleware.UserID(c)

		k, err := kbRepo.FindByID(tenantID, kbID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"code": 4, "message": "knowledge base not found"})
		}

		// KB owner has full access.
		if k.OwnerID == userID {
			return c.Next()
		}

		// Write operations require admin or owner.
		if write {
			return c.Status(403).JSON(fiber.Map{"code": 3, "message": "insufficient permissions"})
		}

		// Read: team-visible KBs are accessible by any tenant member.
		if k.Visibility == kb.VisibilityTeam {
			return c.Next()
		}

		return c.Status(403).JSON(fiber.Map{"code": 3, "message": "insufficient permissions"})
	}
}
