package install

import (
	"context"
	"log"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/internal/rerank"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/internal/user"
	"ollmo/ollmo/pkg/errs"
)

// Service handles first-run system initialization: creating the admin
// tenant/user and seeding default model providers so the platform is
// usable immediately after install.
type Service struct {
	db         *gorm.DB
	users      *user.Repo
	tenants    *tenant.Repo
	llmRepo    *llm.Repo
	embedRepo  *embedding.Repo
	rerankRepo *rerank.Repo
	jwtSecret  string
	jwtExpire  int
}

func NewService(
	db *gorm.DB,
	users *user.Repo,
	tenants *tenant.Repo,
	llmRepo *llm.Repo,
	embedRepo *embedding.Repo,
	rerankRepo *rerank.Repo,
	jwtSecret string,
	jwtExpireHours int,
) *Service {
	return &Service{
		db:         db,
		users:      users,
		tenants:    tenants,
		llmRepo:    llmRepo,
		embedRepo:  embedRepo,
		rerankRepo: rerankRepo,
		jwtSecret:  jwtSecret,
		jwtExpire:  jwtExpireHours,
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
// shared across all three default providers (LLM, embedding, rerank).
// When empty the providers are still created so the user can fill in
// their API key later via Settings.
type InstallInput struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Name         string `json:"name"`
	TenantName   string `json:"tenant_name"`
	ProviderKey  string `json:"provider_key"`
	ProviderName string `json:"provider_name"`
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
			Provider:    "openai",
			Endpoint:    "https://api.siliconflow.cn",
			Model:       "deepseek-ai/DeepSeek-V4-Flash",
			Temperature: 0.7,
			MaxTokens:   2048,
			TopP:        0.9,
		},
		Embedding: EmbeddingDefaults{
			Name:      "BGE Large ZH",
			Endpoint:  "https://api.siliconflow.cn",
			Model:     "BAAI/bge-large-zh-v1.5",
			Dim:       1024,
			BatchSize: 32,
		},
		Rerank: RerankDefaults{
			Name:     "BGE Reranker",
			Endpoint: "https://api.siliconflow.cn",
			Model:    "BAAI/bge-reranker-v2-m3",
			TopN:     10,
		},
	}
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
	providerName := in.ProviderName
	if providerName == "" {
		providerName = "SiliconFlow"
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

	// Seed default providers with the user-provided API key.
	defaults := Defaults()
	apiKey := in.ProviderKey

	llmP := &llm.LLMModel{
		ID:          uuid.NewString(),
		TenantID:    t.ID,
		Name:        defaults.LLM.Name,
		Provider:    defaults.LLM.Provider,
		Endpoint:    defaults.LLM.Endpoint,
		APIKey:      apiKey,
		Model:       defaults.LLM.Model,
		Temperature: defaults.LLM.Temperature,
		MaxTokens:   defaults.LLM.MaxTokens,
		TopP:        defaults.LLM.TopP,
		IsDefault:   true,
		OwnerID:     u.ID,
		Status:      llm.StatusActive,
	}
	if err := s.llmRepo.Create(llmP); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create llm provider", err)
	}

	embedP := &embedding.EmbeddingModel{
		ID:        uuid.NewString(),
		TenantID:  t.ID,
		Name:      defaults.Embedding.Name,
		Endpoint:  defaults.Embedding.Endpoint,
		APIKey:    apiKey,
		Model:     defaults.Embedding.Model,
		Dim:       defaults.Embedding.Dim,
		BatchSize: defaults.Embedding.BatchSize,
		IsDefault: true,
		OwnerID:   u.ID,
		Status:    embedding.StatusActive,
	}
	if err := s.embedRepo.Create(embedP); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create embedding provider", err)
	}

	rerankP := &rerank.RerankModel{
		ID:        uuid.NewString(),
		TenantID:  t.ID,
		Name:      defaults.Rerank.Name,
		Endpoint:  defaults.Rerank.Endpoint,
		APIKey:    apiKey,
		Model:     defaults.Rerank.Model,
		TopN:      defaults.Rerank.TopN,
		IsDefault: true,
		OwnerID:   u.ID,
		Status:    rerank.StatusActive,
	}
	if err := s.rerankRepo.Create(rerankP); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create rerank provider", err)
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
