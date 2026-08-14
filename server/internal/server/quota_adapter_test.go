package server

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/pkg/errs"
)

// newQuotaFixture starts a Lua-capable in-memory Redis (miniredis) plus a
// sqlite tenant table seeded with the given message quotas. The sqlite DB is
// file-backed (temp dir): ":memory:" gives every pooled connection its own
// empty database, which breaks under the test's concurrent readers.
func newQuotaFixture(t *testing.T, msgQuota, userMsgQuota int) *quotaAdapter {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Close the pool before TempDir cleanup; on Windows an open sqlite file
	// cannot be removed, which fails the whole test.
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&tenant.Tenant{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.Create(&tenant.Tenant{
		ID: "tenant-1", Name: "t",
		MessageQuota: msgQuota, UserMessageQuota: userMsgQuota,
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return &quotaAdapter{repo: tenant.NewRepo(db), rdb: rdb}
}

// wantForbidden asserts err is the typed Forbidden error.
func wantForbidden(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected Forbidden error, got nil")
	}
	e, ok := err.(*errs.Error)
	if !ok || e.Code != errs.CodeForbidden {
		t.Fatalf("expected *errs.Error CodeForbidden, got %v", err)
	}
}

func TestTryConsumeMessage_Unlimited(t *testing.T) {
	q := newQuotaFixture(t, -1, -1)
	rem, err := q.TryConsumeMessage("tenant-1", "user-1")
	if err != nil || rem != -1 {
		t.Fatalf("TryConsumeMessage = (%d, %v), want (-1, nil)", rem, err)
	}
	// Unlimited plans must not write any counters.
	if n := q.rdb.DBSize(context.Background()).Val(); n != 0 {
		t.Errorf("expected no redis keys, got %d", n)
	}
}

func TestTryConsumeMessage_TenantCap(t *testing.T) {
	q := newQuotaFixture(t, 3, -1)
	for i := 1; i <= 3; i++ {
		rem, err := q.TryConsumeMessage("tenant-1", "user-1")
		if err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
		if want := 3 - i; rem != want {
			t.Fatalf("consume %d remaining = %d, want %d", i, rem, want)
		}
	}
	_, err := q.TryConsumeMessage("tenant-1", "user-1")
	wantForbidden(t, err)
}

func TestTryConsumeMessage_UserCap(t *testing.T) {
	q := newQuotaFixture(t, -1, 2)
	for i := 0; i < 2; i++ {
		if _, err := q.TryConsumeMessage("tenant-1", "user-1"); err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
	}
	_, err := q.TryConsumeMessage("tenant-1", "user-1")
	wantForbidden(t, err)
}

// TestTryConsumeMessage_TenantCapSharedAcrossUsers verifies the tenant-level
// counter is shared: user-2 is blocked once user-1 exhausted the tenant cap.
func TestTryConsumeMessage_TenantCapSharedAcrossUsers(t *testing.T) {
	q := newQuotaFixture(t, 1, -1)
	if _, err := q.TryConsumeMessage("tenant-1", "user-1"); err != nil {
		t.Fatalf("user-1: %v", err)
	}
	_, err := q.TryConsumeMessage("tenant-1", "user-2")
	wantForbidden(t, err)
}

// TestTryConsumeMessage_AtomicUnderConcurrency is the TOCTOU regression test:
// 50 concurrent consumers against a cap of 10 must admit exactly 10. With the
// old separate GET/INCR pair, every goroutine could pass the check before any
// increment landed and collectively overshoot the quota.
func TestTryConsumeMessage_AtomicUnderConcurrency(t *testing.T) {
	const limit = 10
	const workers = 50
	q := newQuotaFixture(t, limit, -1)

	var wg sync.WaitGroup
	var mu sync.Mutex
	okCount, forbidden := 0, 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := q.TryConsumeMessage("tenant-1", "user-1")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				okCount++
			default:
				if e, ok := err.(*errs.Error); ok && e.Code == errs.CodeForbidden {
					forbidden++
				}
			}
		}()
	}
	wg.Wait()

	if okCount != limit {
		t.Errorf("admitted %d requests, want exactly %d (quota must not overshoot)", okCount, limit)
	}
	if okCount+forbidden != workers {
		t.Errorf("accounted for %d of %d results", okCount+forbidden, workers)
	}

	// The stored counter must equal the limit, proving no overshoot.
	key := fmt.Sprintf("quota:msg:tenant:tenant-1:%s", time.Now().Format("2006-01-02"))
	n, err := q.rdb.Get(context.Background(), key).Int()
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if n != limit {
		t.Errorf("counter = %d, want %d", n, limit)
	}
}
