package provider

import (
	"strings"

	"gorm.io/gorm"

	"ollmo/ollmo/pkg/modelcatalog"
)

// Migrate consolidates per-kind provider cards into unified cards.
// Before: (tenant_id, kind, endpoint) → one card per kind.
// After: (tenant_id, endpoint) → one card for all kinds.
// Idempotent: skips if kind column already dropped.
func Migrate(db *gorm.DB) error {
	// Check if kind column exists.
	var count int64
	db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'model_providers' AND column_name = 'kind'").Scan(&count)
	if count == 0 {
		return nil // already migrated
	}

	// Step 1: Group existing cards by (tenant_id, endpoint).
	type oldCard struct {
		ID        string
		TenantID  string
		Endpoint  string
		CatalogID string
		Name      string
		APIKey    string
		Kind      string
	}
	var cards []oldCard
	if err := db.Table("model_providers").Select("id, tenant_id, endpoint, catalog_id, name, api_key, kind").Find(&cards).Error; err != nil {
		return err
	}

	// Group by (tenant_id, endpoint).
	groups := make(map[string][]oldCard)
	for _, c := range cards {
		key := c.TenantID + "\x00" + c.Endpoint
		groups[key] = append(groups[key], c)
	}

	// Step 2: For each group, pick the card with most models as survivor.
	for _, group := range groups {
		if len(group) <= 1 {
			// Single card, just drop kind.
			if len(group) == 1 {
				db.Exec("UPDATE model_providers SET kind = '' WHERE id = ?", group[0].ID)
			}
			continue
		}

		// Count models per card.
		counts := make(map[string]int64)
		for _, c := range group {
			var n, n2, n3 int64
			db.Table("llm_models").Where("provider_id = ?", c.ID).Count(&n)
			db.Table("embedding_models").Where("provider_id = ?", c.ID).Count(&n2)
			db.Table("rerank_models").Where("provider_id = ?", c.ID).Count(&n3)
			counts[c.ID] = n + n2 + n3
		}

		// Pick survivor (most models, or first if tie).
		survivor := group[0]
		maxCount := counts[survivor.ID]
		for _, c := range group[1:] {
			if counts[c.ID] > maxCount {
				survivor = c
				maxCount = counts[c.ID]
			}
		}

		// Update other cards' models to point to survivor.
		for _, c := range group {
			if c.ID == survivor.ID {
				continue
			}
			db.Table("llm_models").Where("provider_id = ?", c.ID).Update("provider_id", survivor.ID)
			db.Table("embedding_models").Where("provider_id = ?", c.ID).Update("provider_id", survivor.ID)
			db.Table("rerank_models").Where("provider_id = ?", c.ID).Update("provider_id", survivor.ID)
			db.Exec("DELETE FROM model_providers WHERE id = ?", c.ID)
		}

		// Clear kind on survivor.
		db.Exec("UPDATE model_providers SET kind = '' WHERE id = ?", survivor.ID)
	}

	// Step 3: Drop kind column.
	return db.Exec("ALTER TABLE model_providers DROP COLUMN kind").Error
}

// Backfill groups pre-existing model rows under provider cards so the
// settings page shows them after upgrade. Rows are grouped by endpoint;
// a group whose endpoint matches a catalog entry adopts its id/name,
// otherwise a custom card named after the host is created. API keys are
// copied as stored (already encrypted), so no crypto key is needed here.
// Idempotent: rows already bound to a card (provider_id set) are skipped.
func Backfill(db *gorm.DB) error {
	tables := []string{"llm_models", "embedding_models", "rerank_models"}
	selects := map[string][]string{
		"llm_models":       {"id", "tenant_id", "endpoint", "provider", "api_key"},
		"embedding_models": {"id", "tenant_id", "endpoint", "api_key"},
		"rerank_models":    {"id", "tenant_id", "endpoint", "api_key"},
	}
	for _, table := range tables {
		type row struct {
			ID       string
			TenantID string
			Endpoint string
			Provider string
			APIKey   string
		}
		var rows []row
		if err := db.Table(table).
			Select(selects[table]).
			Where("provider_id IS NULL OR provider_id = ''").
			Find(&rows).Error; err != nil {
			return err
		}
		// Cards are tenant-scoped, so the grouping keys on tenant too.
		byKey := map[string][]row{}
		for _, r := range rows {
			k := r.TenantID + "\x00" + r.Endpoint
			byKey[k] = append(byKey[k], r)
		}
		for _, group := range byKey {
			r0 := group[0]
			endpoint := r0.Endpoint
			catalogID, name := matchCatalog(endpoint, r0.Provider)
			p := Provider{
				ID:        NewID(),
				TenantID:  r0.TenantID,
				CatalogID: catalogID,
				Name:      name,
				Endpoint:  endpoint,
				// Copy the group's stored key as-is: it is already encrypted,
				// and the card must keep ownership of it after the rows are
				// grouped (empty card key would clear the rows on save).
				APIKey: r0.APIKey,
			}
			if err := db.Create(&p).Error; err != nil {
				return err
			}
			ids := make([]string, 0, len(group))
			for _, r := range group {
				ids = append(ids, r.ID)
			}
			if err := db.Table(table).Where("id IN ?", ids).
				Update("provider_id", p.ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// matchCatalog resolves a catalog id and display name for an endpoint.
// Preference: exact catalog endpoint match, then the row's legacy provider
// field, then the endpoint host.
func matchCatalog(endpoint, legacyProvider string) (string, string) {
	trimmed := strings.TrimRight(endpoint, "/")
	for _, spec := range modelcatalog.All() {
		if spec.ID == modelcatalog.CustomID {
			continue
		}
		if strings.TrimRight(spec.Endpoint, "/") == trimmed {
			return spec.ID, spec.Name
		}
	}
	if legacyProvider != "" && legacyProvider != modelcatalog.CustomID {
		if spec, ok := modelcatalog.FindByID(legacyProvider); ok && spec.ID != modelcatalog.CustomID {
			return spec.ID, spec.Name
		}
		return legacyProvider, legacyProvider
	}
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	if i := strings.Index(host, "/"); i > 0 {
		host = host[:i]
	}
	return modelcatalog.CustomID, host
}
