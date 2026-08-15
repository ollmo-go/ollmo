package modelcatalog

// Kinds a provider card can serve. Each kind maps to one model table.
const (
	KindChat      = "chat"
	KindEmbedding = "embedding"
	KindRerank    = "rerank"
)

// CustomID marks the user-defined gateway entry.
const CustomID = "custom"

// ModelSpec is one curated model. ContextLength and Note prefill the add
// form; 0 means unknown.
type ModelSpec struct {
	ID            string `json:"id"`
	ContextLength int    `json:"context_length,omitempty"`
	Note          string `json:"note,omitempty"`
}

// ProviderSpec describes one catalog provider: a known vendor with a default
// OpenAI-compatible endpoint and curated model lists per kind, so adding it
// needs only an API key. A provider can serve multiple model kinds.
type ProviderSpec struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Endpoint        string      `json:"endpoint"`
	Note            string      `json:"note,omitempty"`
	ChatModels      []ModelSpec `json:"chat_models,omitempty"`
	EmbeddingModels []ModelSpec `json:"embedding_models,omitempty"`
	RerankModels    []ModelSpec `json:"rerank_models,omitempty"`
}

// providers is the unified catalog: each entry lists all model kinds it supports.
var providers = []ProviderSpec{
	{
		ID:       "deepseek",
		Name:     "DeepSeek",
		Endpoint: "https://api.deepseek.com/v1",
		ChatModels: []ModelSpec{
			{ID: "deepseek-v4-flash", ContextLength: 128 * 1024},
			{ID: "deepseek-v4-pro", ContextLength: 128 * 1024},
		},
	},
	{
		ID:       "openai",
		Name:     "OpenAI",
		Endpoint: "https://api.openai.com/v1",
		ChatModels: []ModelSpec{
			{ID: "gpt-4o-mini", ContextLength: 128 * 1024},
			{ID: "gpt-4o", ContextLength: 128 * 1024},
			{ID: "gpt-4.1-mini", ContextLength: 1024 * 1024},
			{ID: "gpt-4.1", ContextLength: 1024 * 1024},
		},
		EmbeddingModels: []ModelSpec{
			{ID: "text-embedding-3-small"},
			{ID: "text-embedding-3-large"},
		},
	},
	{
		ID:       "zhipu",
		Name:     "Zhipu AI",
		Endpoint: "https://open.bigmodel.cn/api/paas/v4",
		ChatModels: []ModelSpec{
			{ID: "glm-5.2", ContextLength: 128 * 1024},
			{ID: "glm-5", ContextLength: 128 * 1024},
			{ID: "glm-4.5-air", ContextLength: 128 * 1024},
		},
		EmbeddingModels: []ModelSpec{{ID: "embedding-3"}},
		RerankModels:    []ModelSpec{{ID: "rerank"}},
	},
	{
		ID:       "moonshot",
		Name:     "Moonshot AI",
		Endpoint: "https://api.moonshot.cn/v1",
		ChatModels: []ModelSpec{
			{ID: "kimi-k2-0905-preview", ContextLength: 256 * 1024},
			{ID: "moonshot-v1-128k", ContextLength: 128 * 1024},
		},
	},
	{
		ID:       "siliconflow",
		Name:     "SiliconFlow",
		Endpoint: "https://api.siliconflow.cn/v1",
		ChatModels: []ModelSpec{
			{ID: "deepseek-ai/DeepSeek-V3", ContextLength: 64 * 1024},
			{ID: "deepseek-ai/DeepSeek-R1", ContextLength: 64 * 1024},
			{ID: "Qwen/Qwen3-32B", ContextLength: 128 * 1024},
			{ID: "deepseek-ai/DeepSeek-V4-Flash", ContextLength: 128 * 1024},
		},
		EmbeddingModels: []ModelSpec{
			{ID: "BAAI/bge-m3"},
			{ID: "BAAI/bge-large-zh-v1.5"},
			{ID: "netease-youdao/bce-embedding-base_v1"},
		},
		RerankModels: []ModelSpec{
			{ID: "BAAI/bge-reranker-v2-m3"},
			{ID: "netease-youdao/bce-reranker-base_v1"},
		},
	},
	{
		ID:       "ollama",
		Name:     "Ollama (local)",
		Endpoint: "http://localhost:11434/v1",
		Note:     "no_key",
		ChatModels: []ModelSpec{
			{ID: "llama3", ContextLength: 8192},
		},
		EmbeddingModels: []ModelSpec{
			{ID: "bge-m3"},
			{ID: "nomic-embed-text"},
		},
	},
	{
		ID:              "jina",
		Name:            "Jina AI",
		Endpoint:        "https://api.jina.ai/v1",
		EmbeddingModels: []ModelSpec{{ID: "jina-embeddings-v3"}},
		RerankModels:    []ModelSpec{{ID: "jina-reranker-v2-base-multilingual"}},
	},
	{
		ID:       "cohere",
		Name:     "Cohere",
		Endpoint: "https://api.cohere.com/v1",
		RerankModels: []ModelSpec{
			{ID: "rerank-v3.5"},
			{ID: "rerank-multilingual-v3.0"},
		},
	},
	{ID: CustomID, Name: "custom", Endpoint: ""},
}

// All returns the full catalog, custom entry always last.
func All() []ProviderSpec {
	return providers
}

// FindByID resolves a catalog provider spec by id.
func FindByID(id string) (ProviderSpec, bool) {
	for _, p := range providers {
		if p.ID == id {
			return p, true
		}
	}
	return ProviderSpec{}, false
}

// ModelsForKind returns the model list for a specific kind.
func ModelsForKind(spec ProviderSpec, kind string) []ModelSpec {
	switch kind {
	case KindChat:
		return spec.ChatModels
	case KindEmbedding:
		return spec.EmbeddingModels
	case KindRerank:
		return spec.RerankModels
	}
	return nil
}
