package site

import (
	"sync"

	"ollmo/ollmo/pkg/errs"
)

// Service wraps Repo with an in-memory cache. Settings are loaded once at
// startup and refreshed on update. Read-heavy paths (e.g. site name for
// every page load) never hit the database.
type Service struct {
	repo *Repo

	mu    sync.RWMutex
	cache map[string]string
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

// Load fetches all settings from DB and populates the cache. Called once
// at startup.
func (s *Service) Load() error {
	m, err := s.repo.LoadAll()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "load site settings", err)
	}
	s.mu.Lock()
	s.cache = m
	s.mu.Unlock()
	return nil
}

// Get returns a single setting value from cache. Returns empty string if
// the key does not exist.
func (s *Service) Get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[key]
}

// RegistrationAllowed reports whether self-service registration is enabled.
// Registration is allowed only when the setting is explicitly "true"; a
// missing or any other value disables it. This matches the strict check in
// the auth router and the settings page toggle.
func (s *Service) RegistrationAllowed() bool {
	return s.Get(KeyAllowRegistration) == "true"
}

// AutoMemoryEnabled reports whether automatic conversation summarization is
// enabled. Defaults to ON: only an explicit "false" disables it, so existing
// installs without the setting row keep the new behavior active.
func (s *Service) AutoMemoryEnabled() bool {
	return s.Get(KeyAutoMemory) != "false"
}

// All returns a copy of all cached settings.
func (s *Service) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]string, len(s.cache))
	for k, v := range s.cache {
		cp[k] = v
	}
	return cp
}

// Update sets one or more settings and refreshes the cache.
func (s *Service) Update(updates map[string]string) error {
	for k, v := range updates {
		if err := s.repo.Set(k, v); err != nil {
			return errs.Wrap(errs.CodeInternal, "update site setting: "+k, err)
		}
	}
	// Reload cache from DB to stay consistent.
	return s.Load()
}
