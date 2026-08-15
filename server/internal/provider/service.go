package provider

import (
	"context"
	"fmt"
	"strings"

	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/modelcatalog"
)

// Service manages provider cards. Cards hold the credential;
// the model rows they spawn stay in kind-specific tables and keep a copy of
// endpoint/key, so nothing else in the system needs to know about cards.
type Service struct {
	repo        *Repo
	chatStore   KindStore
	embedStore  KindStore
	rerankStore KindStore
}

func NewService(repo *Repo, chatStore, embedStore, rerankStore KindStore) *Service {
	return &Service{
		repo:        repo,
		chatStore:   chatStore,
		embedStore:  embedStore,
		rerankStore: rerankStore,
	}
}

func (s *Service) storeForKind(kind string) (KindStore, error) {
	switch kind {
	case modelcatalog.KindChat, "llm":
		return s.chatStore, nil
	case modelcatalog.KindEmbedding:
		return s.embedStore, nil
	case modelcatalog.KindRerank:
		return s.rerankStore, nil
	}
	return nil, errs.BadRequest("invalid model kind: " + kind)
}

// Card is one provider card with its bound models, as shown in settings.
type Card struct {
	Provider
	HasKey       bool       `json:"has_key"`
	ChatModels   []ModelRef `json:"chat_models"`
	EmbedModels  []ModelRef `json:"embed_models"`
	RerankModels []ModelRef `json:"rerank_models"`
}

// CreateInput adds a provider. CatalogID selects a catalog entry whose
// name/endpoint defaults apply when the caller omits them; custom requires
// an explicit endpoint. Models lands model rows in the same request so
// adding a provider is one pass; a catalog provider defaults to every
// catalog model when Models is omitted.
type CreateInput struct {
	CatalogID    string       `json:"catalog_id"`
	Name         string       `json:"name"`
	Endpoint     string       `json:"endpoint"`
	APIKey       string       `json:"api_key"`
	ChatModels   []ModelInput `json:"chat_models"`
	EmbedModels  []ModelInput `json:"embed_models"`
	RerankModels []ModelInput `json:"rerank_models"`
}

// ModelInput is one model row, at create time or as an edit.
type ModelInput struct {
	Model         string `json:"model"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
}

// UpdateInput edits a card. APIKey nil keeps the stored key (write-only).
type UpdateInput struct {
	Name     *string `json:"name"`
	Endpoint *string `json:"endpoint"`
	APIKey   *string `json:"api_key"`
}

// AddModelInput binds one model row to a card.
type AddModelInput struct {
	Model         string `json:"model"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
}

// UpdateModelInput edits one bound model row's display fields.
type UpdateModelInput struct {
	Name          *string `json:"name"`
	ContextLength *int    `json:"context_length"`
}

// ProbeInput asks an endpoint for its model list using what the FORM
// currently shows — an unsaved endpoint, a key typed but not stored — so
// discovery works before the first save. ProviderID lets an empty key fall
// back to the stored credential of an existing card.
type ProbeInput struct {
	Endpoint   string `json:"endpoint"`
	APIKey     string `json:"api_key"`
	ProviderID string `json:"provider_id"`
}

func (s *Service) Catalog() []modelcatalog.ProviderSpec {
	return modelcatalog.All()
}

func (s *Service) List(tenantID string) ([]Card, error) {
	provs, err := s.repo.List(tenantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list providers", err)
	}
	cards := make([]Card, 0, len(provs))
	for _, p := range provs {
		chatModels, err := s.chatStore.ListModels(tenantID, p.ID)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "list chat models", err)
		}
		embedModels, err := s.embedStore.ListModels(tenantID, p.ID)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "list embed models", err)
		}
		rerankModels, err := s.rerankStore.ListModels(tenantID, p.ID)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "list rerank models", err)
		}
		cards = append(cards, Card{
			Provider:     *p,
			HasKey:       p.APIKey != "",
			ChatModels:   chatModels,
			EmbedModels:  embedModels,
			RerankModels: rerankModels,
		})
	}
	return cards, nil
}

func (s *Service) Create(tenantID, ownerID string, in CreateInput) (*Card, error) {
	if in.CatalogID == "" {
		in.CatalogID = modelcatalog.CustomID
	}
	spec, inCatalog := modelcatalog.FindByID(in.CatalogID)
	if !inCatalog {
		return nil, errs.BadRequest("unknown catalog provider")
	}
	name := in.Name
	if name == "" || name == modelcatalog.CustomID {
		name = spec.Name
	}
	endpoint := in.Endpoint
	if endpoint == "" {
		endpoint = spec.Endpoint
	}
	if endpoint == "" {
		return nil, errs.BadRequest("endpoint is required")
	}
	if err := clients.ValidateEndpoint(endpoint); err != nil {
		return nil, errs.BadRequest(err.Error())
	}
	// One card per endpoint: a provider is identified by its base URL, so
	// the same endpoint cannot be added twice.
	if existing, err := s.repo.FindByEndpoint(tenantID, endpoint); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "check provider endpoint", err)
	} else if existing != nil {
		return nil, errs.Conflict("provider with this endpoint already exists: " + existing.Name)
	}
	p := &Provider{
		ID:        NewID(),
		TenantID:  tenantID,
		CatalogID: in.CatalogID,
		Name:      name,
		Endpoint:  endpoint,
		APIKey:    in.APIKey,
	}
	if err := s.repo.Create(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create provider", err)
	}

	// Create models for each kind, defaulting to catalog if not specified.
	chatModels := in.ChatModels
	if chatModels == nil && inCatalog {
		for _, m := range spec.ChatModels {
			chatModels = append(chatModels, ModelInput{Model: m.ID, ContextLength: m.ContextLength})
		}
	}
	embedModels := in.EmbedModels
	if embedModels == nil && inCatalog {
		for _, m := range spec.EmbeddingModels {
			embedModels = append(embedModels, ModelInput{Model: m.ID})
		}
	}
	rerankModels := in.RerankModels
	if rerankModels == nil && inCatalog {
		for _, m := range spec.RerankModels {
			rerankModels = append(rerankModels, ModelInput{Model: m.ID})
		}
	}

	chatRefs := make([]ModelRef, 0, len(chatModels))
	for _, m := range chatModels {
		if strings.TrimSpace(m.Model) == "" {
			continue
		}
		ref, err := s.chatStore.CreateModel(tenantID, ownerID, p.ID, p, m)
		if err != nil {
			_ = s.repo.Delete(tenantID, p.ID)
			return nil, errs.Wrap(errs.CodeBadRequest, "create chat model", err)
		}
		chatRefs = append(chatRefs, ref)
	}

	embedRefs := make([]ModelRef, 0, len(embedModels))
	for _, m := range embedModels {
		if strings.TrimSpace(m.Model) == "" {
			continue
		}
		ref, err := s.embedStore.CreateModel(tenantID, ownerID, p.ID, p, m)
		if err != nil {
			_ = s.repo.Delete(tenantID, p.ID)
			return nil, errs.Wrap(errs.CodeBadRequest, "create embed model", err)
		}
		embedRefs = append(embedRefs, ref)
	}

	rerankRefs := make([]ModelRef, 0, len(rerankModels))
	for _, m := range rerankModels {
		if strings.TrimSpace(m.Model) == "" {
			continue
		}
		ref, err := s.rerankStore.CreateModel(tenantID, ownerID, p.ID, p, m)
		if err != nil {
			_ = s.repo.Delete(tenantID, p.ID)
			return nil, errs.Wrap(errs.CodeBadRequest, "create rerank model", err)
		}
		rerankRefs = append(rerankRefs, ref)
	}

	return &Card{
		Provider:     *p,
		HasKey:       p.APIKey != "",
		ChatModels:   chatRefs,
		EmbedModels:  embedRefs,
		RerankModels: rerankRefs,
	}, nil
}

func (s *Service) Update(tenantID, id string, in UpdateInput) (*Card, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	credsChanged := false
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.Endpoint != nil && *in.Endpoint != "" && *in.Endpoint != p.Endpoint {
		if err := clients.ValidateEndpoint(*in.Endpoint); err != nil {
			return nil, errs.BadRequest(err.Error())
		}
		// Reject changing to an endpoint already owned by another card.
		if existing, err := s.repo.FindByEndpoint(tenantID, *in.Endpoint); err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "check provider endpoint", err)
		} else if existing != nil && existing.ID != id {
			return nil, errs.Conflict("provider with this endpoint already exists: " + existing.Name)
		}
		p.Endpoint = *in.Endpoint
		credsChanged = true
	}
	if in.APIKey != nil && *in.APIKey != p.APIKey {
		p.APIKey = *in.APIKey
		credsChanged = true
	}
	if err := s.repo.Save(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "save provider", err)
	}
	// Rows carry their own copy of the credentials; re-sync only when they
	// actually changed so an unrelated rename never rewrites row secrets.
	if credsChanged {
		if err := s.chatStore.UpdateCreds(tenantID, id, p.Endpoint, p.APIKey); err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "sync chat credentials", err)
		}
		if err := s.embedStore.UpdateCreds(tenantID, id, p.Endpoint, p.APIKey); err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "sync embed credentials", err)
		}
		if err := s.rerankStore.UpdateCreds(tenantID, id, p.Endpoint, p.APIKey); err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "sync rerank credentials", err)
		}
	}
	chatModels, _ := s.chatStore.ListModels(tenantID, id)
	embedModels, _ := s.embedStore.ListModels(tenantID, id)
	rerankModels, _ := s.rerankStore.ListModels(tenantID, id)
	return &Card{
		Provider:     *p,
		HasKey:       p.APIKey != "",
		ChatModels:   chatModels,
		EmbedModels:  embedModels,
		RerankModels: rerankModels,
	}, nil
}

// Delete removes the card and every model row bound to it.
func (s *Service) Delete(tenantID, id string) error {
	if _, err := s.repo.FindByID(tenantID, id); err != nil {
		return err
	}
	// Refuse to delete a provider while any of its bound models is still
	// referenced by a knowledge base or an agent graph.
	groups := []struct {
		kind  string
		store KindStore
	}{
		{modelcatalog.KindChat, s.chatStore},
		{modelcatalog.KindEmbedding, s.embedStore},
		{modelcatalog.KindRerank, s.rerankStore},
	}
	for _, g := range groups {
		models, err := g.store.ListModels(tenantID, id)
		if err != nil {
			return errs.Wrap(errs.CodeInternal, "list provider models", err)
		}
		for _, m := range models {
			if err := s.checkModelInUse(tenantID, g.kind, m.ID); err != nil {
				return err
			}
		}
	}
	if err := s.chatStore.DeleteAll(tenantID, id); err != nil {
		return errs.Wrap(errs.CodeInternal, "delete chat models", err)
	}
	if err := s.embedStore.DeleteAll(tenantID, id); err != nil {
		return errs.Wrap(errs.CodeInternal, "delete embed models", err)
	}
	if err := s.rerankStore.DeleteAll(tenantID, id); err != nil {
		return errs.Wrap(errs.CodeInternal, "delete rerank models", err)
	}
	if err := s.repo.Delete(tenantID, id); err != nil {
		return err
	}
	return nil
}

// checkModelInUse returns a conflict error when a model row is still pinned
// by a knowledge base (embedding) or wired into an agent graph (chat/rerank).
func (s *Service) checkModelInUse(tenantID, kind, modelID string) error {
	switch kind {
	case modelcatalog.KindEmbedding:
		n, err := s.repo.CountKBsByEmbedding(tenantID, modelID)
		if err != nil {
			return errs.Wrap(errs.CodeInternal, "check knowledge base references", err)
		}
		if n > 0 {
			return errs.Conflict(fmt.Sprintf("model is in use by %d knowledge base(s)", n))
		}
	default:
		n, err := s.repo.CountAgentUsages(tenantID, modelID)
		if err != nil {
			return errs.Wrap(errs.CodeInternal, "check agent references", err)
		}
		if n > 0 {
			return errs.Conflict(fmt.Sprintf("model is in use by %d agent(s)", n))
		}
	}
	return nil
}

// Discover lists the models the provider's endpoint advertises, using the
// stored credential. Advisory only; nothing is persisted.
func (s *Service) Discover(ctx context.Context, tenantID, id string) ([]clients.DiscoveredModel, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	models, err := clients.ListModels(ctx, p.Endpoint, p.APIKey)
	if err != nil {
		return nil, errs.Wrap(errs.CodeBadRequest, "discover models", err)
	}
	return models, nil
}

// AddModel binds one model row to the card, copying the card's endpoint and
// key so the row is self-contained. Adding an already-bound model is a
// no-op returning the existing row.
func (s *Service) AddModel(tenantID, ownerID, id, kind string, in AddModelInput) (ModelRef, error) {
	if strings.TrimSpace(in.Model) == "" {
		return ModelRef{}, errs.BadRequest("model is required")
	}
	store, err := s.storeForKind(kind)
	if err != nil {
		return ModelRef{}, err
	}
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return ModelRef{}, err
	}
	existing, err := store.ListModels(tenantID, id)
	if err != nil {
		return ModelRef{}, errs.Wrap(errs.CodeInternal, "list provider models", err)
	}
	for _, m := range existing {
		if m.Model == in.Model {
			return m, nil
		}
	}
	ref, err := store.CreateModel(tenantID, ownerID, id, p, ModelInput{
		Model:         in.Model,
		Name:          in.Name,
		ContextLength: in.ContextLength,
	})
	if err != nil {
		return ModelRef{}, errs.Wrap(errs.CodeBadRequest, "add provider model", err)
	}
	return ref, nil
}

// RemoveModel deletes one bound model row.
func (s *Service) RemoveModel(tenantID, id, modelID, kind string) error {
	store, err := s.storeForKind(kind)
	if err != nil {
		return err
	}
	models, err := store.ListModels(tenantID, id)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "list provider models", err)
	}
	for _, m := range models {
		if m.ID == modelID {
			if err := s.checkModelInUse(tenantID, kind, modelID); err != nil {
				return err
			}
			return store.DeleteModel(tenantID, modelID)
		}
	}
	return errs.NotFound("model not found under provider")
}

// UpdateModel edits one bound model row's display name / context length.
func (s *Service) UpdateModel(tenantID, id, modelID, kind string, in UpdateModelInput) (ModelRef, error) {
	store, err := s.storeForKind(kind)
	if err != nil {
		return ModelRef{}, err
	}
	models, err := store.ListModels(tenantID, id)
	if err != nil {
		return ModelRef{}, errs.Wrap(errs.CodeInternal, "list provider models", err)
	}
	for _, m := range models {
		if m.ID != modelID {
			continue
		}
		if in.Name != nil {
			m.Name = strings.TrimSpace(*in.Name)
		}
		if in.ContextLength != nil {
			m.ContextLength = *in.ContextLength
		}
		if err := store.UpdateModel(tenantID, m); err != nil {
			return ModelRef{}, errs.Wrap(errs.CodeInternal, "update provider model", err)
		}
		return m, nil
	}
	return ModelRef{}, errs.NotFound("model not found under provider")
}

// Probe lists the models an arbitrary endpoint advertises, using form-state
// values (an unsaved endpoint, a freshly typed key). Advisory only.
func (s *Service) Probe(ctx context.Context, tenantID string, in ProbeInput) ([]clients.DiscoveredModel, error) {
	if in.Endpoint == "" {
		return nil, errs.BadRequest("endpoint is required")
	}
	if err := clients.ValidateEndpoint(in.Endpoint); err != nil {
		return nil, errs.BadRequest(err.Error())
	}
	key := in.APIKey
	if key == "" && in.ProviderID != "" {
		if p, err := s.repo.FindByID(tenantID, in.ProviderID); err == nil {
			key = p.APIKey
		}
	}
	models, err := clients.ListModels(ctx, in.Endpoint, key)
	if err != nil {
		return nil, errs.Wrap(errs.CodeBadRequest, "discover models", err)
	}
	return models, nil
}
