package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"ollmo/ollmo/internal/config"
	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/pkg/errs"
)

// quotaAdapter bridges tenant.Repo to doc.QuotaChecker. It reads the tenant's
// quota limits and current usage counts to enforce document/vector limits.
type quotaAdapter struct {
	repo *tenant.Repo
	rdb  *redis.Client
}

func (q *quotaAdapter) CheckDocQuota(tenantID string) error {
	t, err := q.repo.FindByID(tenantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "find tenant for quota", err)
	}
	count, err := q.repo.CountDocs(tenantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "count docs for quota", err)
	}
	if count >= int64(t.DocQuota) {
		return errs.Forbidden("document quota exceeded")
	}
	return nil
}

// CheckVectorDelta verifies that adding delta more vectors would not exceed
// the tenant's vector quota. Used by the embed worker before upserting.
func (q *quotaAdapter) CheckVectorDelta(tenantID string, delta int) error {
	t, err := q.repo.FindByID(tenantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "find tenant for quota", err)
	}
	count, err := q.repo.CountChunks(tenantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "count chunks for quota", err)
	}
	if int64(delta)+count > int64(t.VectorQuota) {
		return errs.Forbidden("vector quota would be exceeded")
	}
	return nil
}

// tryConsumeScript atomically checks both daily counters against their limits
// and increments them only when both are within bounds. Redis executes Lua
// scripts atomically, which closes the check-then-increment race where
// concurrent requests could each pass the check and collectively exceed the
// quota (TOCTOU).
//
// KEYS[1] tenant counter, KEYS[2] user counter
// ARGV[1] tenant limit, ARGV[2] user limit, ARGV[3] ttl seconds
// Returns {status, value}: status 0 = consumed (value = effective remaining),
// -1 = tenant quota exceeded, -2 = user quota exceeded. Limits < 0 mean
// unlimited and skip that counter entirely.
var tryConsumeScript = redis.NewScript(`
local tlimit = tonumber(ARGV[1])
local ulimit = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
if tlimit >= 0 then
  local n = tonumber(redis.call('GET', KEYS[1]) or '0')
  if n >= tlimit then return {-1, 0} end
end
if ulimit >= 0 then
  local n = tonumber(redis.call('GET', KEYS[2]) or '0')
  if n >= ulimit then return {-2, 0} end
end
local trem, urem = -1, -1
if tlimit >= 0 then
  local n = redis.call('INCR', KEYS[1])
  if n == 1 then redis.call('EXPIRE', KEYS[1], ttl) end
  trem = tlimit - n
end
if ulimit >= 0 then
  local n = redis.call('INCR', KEYS[2])
  if n == 1 then redis.call('EXPIRE', KEYS[2], ttl) end
  urem = ulimit - n
end
local rem = trem
if rem < 0 or (urem >= 0 and urem < rem) then rem = urem end
return {0, rem}
`)

// TryConsumeMessage verifies both daily caps and increments the tenant and
// user counters in a single atomic Redis operation. Replaces the separate
// CheckMessageQuota + IncrMessageUsage pair, whose GET/INCR gap allowed
// concurrent requests to overshoot the quota.
func (q *quotaAdapter) TryConsumeMessage(tenantID, userID string) (int, error) {
	t, err := q.repo.FindByID(tenantID)
	if err != nil {
		return 0, errs.Wrap(errs.CodeInternal, "find tenant for msg quota", err)
	}
	if t.MessageQuota < 0 && t.UserMessageQuota < 0 {
		return -1, nil
	}
	ctx := context.Background()
	date := time.Now().Format("2006-01-02")
	res, err := tryConsumeScript.Run(ctx, q.rdb,
		[]string{
			fmt.Sprintf("quota:msg:tenant:%s:%s", tenantID, date),
			fmt.Sprintf("quota:msg:user:%s:%s", userID, date),
		},
		t.MessageQuota, t.UserMessageQuota, int((25*time.Hour).Seconds()),
	).Result()
	if err != nil {
		return 0, errs.Wrap(errs.CodeInternal, "consume msg quota", err)
	}
	parts, ok := res.([]interface{})
	if !ok || len(parts) != 2 {
		return 0, errs.Wrap(errs.CodeInternal, "consume msg quota", fmt.Errorf("unexpected script result %v", res))
	}
	status, _ := parts[0].(int64)
	switch status {
	case -1:
		return 0, errs.Forbidden("daily tenant message quota exceeded")
	case -2:
		return 0, errs.Forbidden("daily user message quota exceeded")
	}
	value, _ := parts[1].(int64)
	return int(value), nil
}

// MessageUsage returns (effectiveRemaining, userLimit) for display on the
// chat input. effectiveRemaining is the smaller of the tenant and user
// remaining counts (clamped to >=0; -1 only when both are unlimited).
// userLimit is the per-user cap (-1 unlimited), used as the display total.
func (q *quotaAdapter) MessageUsage(tenantID, userID string) (int, int, error) {
	t, err := q.repo.FindByID(tenantID)
	if err != nil {
		return 0, 0, err
	}
	if t.MessageQuota < 0 && t.UserMessageQuota < 0 {
		return -1, -1, nil
	}
	ctx := context.Background()
	date := time.Now().Format("2006-01-02")

	tenantRem := -1
	if t.MessageQuota >= 0 {
		n, _ := q.rdb.Get(ctx, fmt.Sprintf("quota:msg:tenant:%s:%s", tenantID, date)).Int()
		tenantRem = t.MessageQuota - n
	}
	userRem := -1
	if t.UserMessageQuota >= 0 {
		n, _ := q.rdb.Get(ctx, fmt.Sprintf("quota:msg:user:%s:%s", userID, date)).Int()
		userRem = t.UserMessageQuota - n
	}

	rem := effectiveRemaining(tenantRem, userRem)
	if rem < 0 {
		rem = 0
	}
	userLimit := t.UserMessageQuota
	// When the user cap is unlimited but the tenant cap isn't, show the tenant
	// limit as the display total so the number is meaningful.
	if userLimit < 0 && t.MessageQuota >= 0 {
		userLimit = t.MessageQuota
	}
	return rem, userLimit, nil
}

// TenantMessageUsage returns (used, limit) for the tenant's daily message
// counter. Used by the backend usage page. limit=-1 means unlimited.
func (q *quotaAdapter) TenantMessageUsage(tenantID string) (int, int, error) {
	t, err := q.repo.FindByID(tenantID)
	if err != nil {
		return 0, 0, err
	}
	if t.MessageQuota < 0 {
		return 0, -1, nil
	}
	key := fmt.Sprintf("quota:msg:tenant:%s:%s", tenantID, time.Now().Format("2006-01-02"))
	n, err := q.rdb.Get(context.Background(), key).Int()
	if err != nil && err != redis.Nil {
		return 0, 0, err
	}
	return n, t.MessageQuota, nil
}

// TenantMessageUsageBatch returns today's message usage for multiple tenants
// in a single Redis MGET call. Used by the super-admin tenant list to avoid
// one Redis round-trip per tenant.
func (q *quotaAdapter) TenantMessageUsageBatch(tenantIDs []string) map[string]int {
	return q.mgetCounts("quota:msg:tenant", tenantIDs)
}

// UserMessageUsageBatch returns today's message usage for multiple users in a
// single Redis MGET call. Used by the member list to avoid one round-trip
// per user.
func (q *quotaAdapter) UserMessageUsageBatch(userIDs []string) map[string]int {
	return q.mgetCounts("quota:msg:user", userIDs)
}

// mgetCounts reads today's counters for the given IDs under a key prefix in
// one Redis MGET call and returns the parsed counts keyed by ID.
func (q *quotaAdapter) mgetCounts(prefix string, ids []string) map[string]int {
	out := make(map[string]int, len(ids))
	if len(ids) == 0 {
		return out
	}
	date := time.Now().Format("2006-01-02")
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = fmt.Sprintf("%s:%s:%s", prefix, id, date)
	}
	vals, err := q.rdb.MGet(context.Background(), keys...).Result()
	if err != nil {
		return out
	}
	for i, v := range vals {
		if s, ok := v.(string); ok {
			if n, err := strconv.Atoi(s); err == nil {
				out[ids[i]] = n
			}
		}
	}
	return out
}

// effectiveRemaining returns the smaller of two remaining counts where -1
// means unlimited.
func effectiveRemaining(a, b int) int {
	if a < 0 {
		return b
	}
	if b < 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// quotaConfigAdapter bridges config.QuotaConfig to auth.QuotaResolver.
type quotaConfigAdapter struct {
	cfg config.QuotaConfig
}

func (a *quotaConfigAdapter) QuotasFor(plan string) (int, int, int, int) {
	p := a.cfg.QuotasFor(plan)
	return p.DocQuota, p.VectorQuota, p.MessageQuota, p.UserMessageQuota
}

// embeddingResolverAdapter bridges embedding.Repo to kb.EmbeddingResolver so
// the kb package can validate an embedding_model_id without importing the
// embedding package (which would create a circular dependency via doc).
type embeddingResolverAdapter struct {
	repo *embedding.Repo
}

func (a *embeddingResolverAdapter) Resolve(tenantID, id string) (int, error) {
	p, err := a.repo.FindByID(tenantID, id)
	if err != nil {
		return 0, err
	}
	return p.Dim, nil
}
