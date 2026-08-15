package install

import (
	"bytes"
	"context"
	"log"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ollmo/ollmo/internal/agent"
	"ollmo/ollmo/internal/doc"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/internal/provider"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/internal/user"
	"ollmo/ollmo/pkg/errs"
)

// Service handles first-run system initialization: creating the admin
// tenant/user, seeding default model providers, and provisioning a demo
// knowledge base (with a standard-template agent canvas and a demo document)
// so the platform is usable immediately after install.
type Service struct {
	db        *gorm.DB
	users     *user.Repo
	tenants   *tenant.Repo
	providers *provider.Service
	kbs       *kb.Service
	agents    *agent.Service
	docs      *doc.Service
	jwtSecret string
	jwtExpire int
}

func NewService(
	db *gorm.DB,
	users *user.Repo,
	tenants *tenant.Repo,
	providers *provider.Service,
	kbs *kb.Service,
	agents *agent.Service,
	docs *doc.Service,
	jwtSecret string,
	jwtExpireHours int,
) *Service {
	return &Service{
		db:        db,
		users:     users,
		tenants:   tenants,
		providers: providers,
		kbs:       kbs,
		agents:    agents,
		docs:      docs,
		jwtSecret: jwtSecret,
		jwtExpire: jwtExpireHours,
	}
}

// Status reports whether the system has been initialized (at least one
// user exists). The frontend uses this to decide whether to show the
// install wizard or the normal login page.
type Status struct {
	Installed bool `json:"installed"`
}

func (s *Service) Status(ctx context.Context) (*Status, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&user.User{}).Count(&count).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "count users", err)
	}
	return &Status{Installed: count > 0}, nil
}

// InstallInput is the body for POST /install. AdminFields create the
// first tenant + user; ProviderKey is optional — when provided it is
// shared across the three seeded SiliconFlow provider cards (LLM,
// embedding, rerank). When empty the cards are still created so the
// user can fill in their API key later via Settings.
type InstallInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Name        string `json:"name"`
	TenantName  string `json:"tenant_name"`
	ProviderKey string `json:"provider_key"`
}

// DefaultProviderConfigs returns the pre-filled provider configurations
// so the frontend can display them before the user confirms. API keys
// are omitted (desensitized).
type DefaultProviderConfigs struct {
	LLM       LLMDefaults       `json:"llm"`
	Embedding EmbeddingDefaults `json:"embedding"`
	Rerank    RerankDefaults    `json:"rerank"`
}

type LLMDefaults struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	Endpoint    string  `json:"endpoint"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	TopP        float64 `json:"top_p"`
}

type EmbeddingDefaults struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	Model     string `json:"model"`
	Dim       int    `json:"dim"`
	BatchSize int    `json:"batch_size"`
}

type RerankDefaults struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	TopN     int    `json:"top_n"`
}

// Defaults returns the pre-filled provider configurations derived from
// the current working setup. API keys are intentionally omitted.
func Defaults() DefaultProviderConfigs {
	return DefaultProviderConfigs{
		LLM: LLMDefaults{
			Name:        "DeepSeek V4",
			Provider:    "siliconflow",
			Endpoint:    "https://api.siliconflow.cn/v1",
			Model:       "deepseek-ai/DeepSeek-V4-Flash",
			Temperature: 0.7,
			MaxTokens:   2048,
			TopP:        0.9,
		},
		Embedding: EmbeddingDefaults{
			Name:      "BGE Large ZH",
			Endpoint:  "https://api.siliconflow.cn/v1",
			Model:     "BAAI/bge-large-zh-v1.5",
			Dim:       1024,
			BatchSize: 32,
		},
		Rerank: RerankDefaults{
			Name:     "BGE Reranker",
			Endpoint: "https://api.siliconflow.cn/v1",
			Model:    "BAAI/bge-reranker-v2-m3",
			TopN:     10,
		},
	}
}

// Demo knowledge base provisioned during install so the platform is usable
// immediately: a standard-template agent canvas and a small demo document
// whose content covers the product itself for try-out questions.
const (
	demoKbName     = "演示知识库"
	demoKbDesc     = "安装时自动创建的演示知识库，内置标准智能体画布与示例文档，可直接提问体验。"
	demoDocName    = "产品介绍.md"
	demoDocMime    = "text/markdown"
	demoDocSize    = int64(len(demoDocContent))
	demoDocContent = `# Ollmo 知识库问答平台

Ollmo 是一个开箱即用的企业级知识库问答平台（RAG），帮助你基于自有文档构建智能问答助手。

## 核心能力

- **知识库管理**：支持批量上传 Markdown、PDF、Word、TXT 等多种格式文档，自动完成解析、切片与向量化。
- **智能体画布**：通过可视化节点（意图分类、检索、条件分支、LLM、直接回复）编排问答流程，无需编写代码。
- **混合检索**：结合向量语义检索与关键词精确匹配，显著提升召回效果。
- **多模型接入**：支持 SiliconFlow、Ollama 等主流大模型供应商，一条 API Key 即可接入对话、向量化与重排模型。

## 快速开始

1. 在"知识库"页面新建知识库并上传文档。
2. 在"智能体"画布中选择模板或自由编排节点。
3. 回到首页，选择该知识库即可开始对话。

## 常见问题

问：安装完成后如何开始使用？
答：安装程序会自动创建一个演示知识库并上传示例文档，你只需在首页聊天框输入问题即可体验完整问答流程。

问：支持哪些文档格式？
答：当前支持 Markdown、PDF、Word、TXT 以及 CSV（问答对）等格式。

问：如何接入自己的模型？
答：在"设置 → 模型"中填入 SiliconFlow 等供应商的 API Key，或添加自定义模型端点。
`
)

// seedDemo provisions the demo knowledge base, its standard-template agent
// canvas, and the demo document. It runs after the provider card exists so
// the KB pins the freshly created embedding model and the agent graph
// resolves the LLM/rerank defaults. Failures are non-fatal: they are logged
// so a broken external dependency (MinIO, asynq, ...) never blocks an
// otherwise successful install.
func (s *Service) seedDemo(ctx context.Context, tenantID, ownerID string, card *provider.Card) {
	// KB must pin an embedding model id; the first model on the seeded card
	// is the tenant default.
	embedID := ""
	llmID := ""
	rerankID := ""
	if len(card.EmbedModels) > 0 {
		embedID = card.EmbedModels[0].ID
	}
	if len(card.ChatModels) > 0 {
		llmID = card.ChatModels[0].ID
	}
	if len(card.RerankModels) > 0 {
		rerankID = card.RerankModels[0].ID
	}
	if embedID == "" {
		log.Printf("[install] skip demo seed: no embedding model on provider card")
		return
	}

	k, err := s.kbs.Create(ctx, tenantID, ownerID, kb.CreateInput{
		Name:             demoKbName,
		Description:      demoKbDesc,
		EmbeddingModelID: embedID,
	})
	if err != nil {
		log.Printf("[install] seed demo kb failed: %v", err)
		return
	}
	// Team visibility exposes the demo KB to foreground chat users.
	if err := s.kbs.SetVisibility(ctx, tenantID, k.ID, kb.VisibilityTeam); err != nil {
		log.Printf("[install] set demo kb visibility failed: %v", err)
	}

	if _, err := s.agents.Save(ctx, tenantID, k.ID, agent.BuildStandardDefinition(llmID, rerankID)); err != nil {
		log.Printf("[install] seed demo agent failed: %v", err)
	}

	if s.docs != nil {
		if _, err := s.docs.Upload(ctx, tenantID, ownerID, k.ID, demoDocName, demoDocMime, demoDocSize, bytes.NewReader([]byte(demoDocContent))); err != nil {
			log.Printf("[install] seed demo document failed: %v", err)
		}
	}
	log.Printf("[install] demo knowledge base provisioned: kb=%s agent=standard doc=%s", k.ID, demoDocName)
}

// Install creates the first admin tenant, user, and default model
// providers. It refuses to run if any user already exists. Returns a
// JWT so the caller can start using the system immediately.
func (s *Service) Install(ctx context.Context, in InstallInput) (*TokenResponse, error) {
	if in.Email == "" || in.Password == "" {
		return nil, errs.BadRequest("email and password are required")
	}

	// Refuse if already installed.
	st, err := s.Status(ctx)
	if err != nil {
		return nil, err
	}
	if st.Installed {
		return nil, errs.Conflict("system is already installed")
	}

	tenantName := in.TenantName
	if tenantName == "" {
		tenantName = in.Email + "'s workspace"
	}

	t := &tenant.Tenant{
		ID:               uuid.NewString(),
		Name:             tenantName,
		Plan:             "free",
		DocQuota:         100,
		VectorQuota:      10000,
		MessageQuota:     100,
		UserMessageQuota: 20,
	}
	if err := s.tenants.Create(t); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create tenant", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "hash password", err)
	}
	u := &user.User{
		ID:           uuid.NewString(),
		TenantID:     t.ID,
		Email:        in.Email,
		PasswordHash: string(hash),
		Name:         ifEmpty(in.Name, in.Email),
		Role:         "admin",
		Status:       "active",
		IsSuperAdmin: true,
	}
	if err := s.users.Create(u); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create user", err)
	}

	member := &tenant.TenantMember{
		ID:       uuid.NewString(),
		TenantID: t.ID,
		UserID:   u.ID,
		Role:     tenant.RoleOwner,
		Current:  true,
	}
	if err := s.tenants.CreateMember(member); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create tenant member", err)
	}

	// Seed one SiliconFlow provider card via the unified provider service,
	// so installed data lands in the same provider-card shape the settings
	// UI manages. An empty key still creates the card; the user fills the
	// key later via Settings.
	defaults := Defaults()
	apiKey := in.ProviderKey

	card, err := s.providers.Create(t.ID, u.ID, provider.CreateInput{
		CatalogID: "siliconflow",
		APIKey:    apiKey,
		ChatModels: []provider.ModelInput{
			{Model: defaults.LLM.Model, ContextLength: 128 * 1024},
		},
		EmbedModels: []provider.ModelInput{
			{Model: defaults.Embedding.Model},
		},
		RerankModels: []provider.ModelInput{
			{Model: defaults.Rerank.Model},
		},
	})
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create provider", err)
	}

	// Provision the demo knowledge base so the install is usable out of the
	// box. Best-effort: a failure here is logged and never blocks install.
	if s.kbs != nil && s.agents != nil {
		s.seedDemo(ctx, t.ID, u.ID, card)
	}

	token, err := middleware.IssueToken(s.jwtSecret, u.ID, u.TenantID, u.Role, u.IsSuperAdmin, s.jwtExpire)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "sign jwt", err)
	}

	log.Printf("[install] system initialized: tenant=%s user=%s email=%s", t.ID, u.ID, u.Email)
	return &TokenResponse{
		Token:     token,
		ExpiresIn: s.jwtExpire * 3600,
		UserID:    u.ID,
		TenantID:  u.TenantID,
	}, nil
}

type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	UserID    string `json:"user_id"`
	TenantID  string `json:"tenant_id"`
}

func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
