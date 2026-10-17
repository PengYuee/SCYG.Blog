//go:build integration

package publicsearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
)

func redisTestConfig(t *testing.T, ttl string) config.PublicSearch {
	t.Helper()
	url := os.Getenv("PUBLIC_SEARCH_TEST_REDIS_URL")
	if url == "" {
		t.Fatal("PUBLIC_SEARCH_TEST_REDIS_URL must point to an isolated real Redis")
	}
	path := filepath.Join(t.TempDir(), "runtime.yaml")
	body := fmt.Sprintf("public_search:\n  redis_url: %q\n  prefix: scyg:test:%s\n  lease_ttl: %s\n  lease_renew_interval: 40ms\n", url, uuid.NewString(), ttl)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{File: path, DisableEnvironment: true})
	if err != nil {
		t.Fatal(err)
	}
	return cfg.PublicSearch()
}

func TestRedisPublicBucketsAndConnectionLeasesAcrossReplicas(t *testing.T) {
	cfg := redisTestConfig(t, "120ms")
	ctx := context.Background()
	first, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	if allowed, _, err := first.Allow(ctx, "search", "127.0.0.1"); err != nil || !allowed {
		t.Fatalf("first search: %v", err)
	}
	if allowed, retry, err := second.Allow(ctx, "search", "127.0.0.1"); err != nil || allowed || retry <= 0 {
		t.Fatalf("replica bypassed shared bucket: %v %v", allowed, err)
	}
	if allowed, _, err := second.Allow(ctx, "session", "127.0.0.1"); err != nil || !allowed {
		t.Fatalf("independent issuance bucket: %v", err)
	}
	one, allowed, err := first.Acquire(ctx, "same-run")
	if err != nil || !allowed {
		t.Fatalf("first lease: %v", err)
	}
	defer one.Release()
	two, allowed, err := second.Acquire(ctx, "same-run")
	if err != nil || !allowed {
		t.Fatalf("second lease: %v", err)
	}
	defer two.Release()
	if _, allowed, err := second.Acquire(ctx, "same-run"); err != nil || allowed {
		t.Fatalf("third connection admitted: %v", err)
	}
	one.Release()
	if valid, err := one.Renew(ctx); err != nil || valid {
		t.Fatalf("released owner renewed: %v", err)
	}
	three, allowed, err := second.Acquire(ctx, "same-run")
	if err != nil || !allowed {
		t.Fatalf("released slot not reusable: %v", err)
	}
	defer three.Release()
	time.Sleep(180 * time.Millisecond)
	if valid, err := two.Renew(ctx); err != nil || valid {
		t.Fatalf("expired owner resurrected: %v", err)
	}
	recovered, allowed, err := first.Acquire(ctx, "same-run")
	if err != nil || !allowed {
		t.Fatalf("expired slots not recovered: %v", err)
	}
	defer recovered.Release()
	if valid, err := recovered.Renew(ctx); err != nil || !valid {
		t.Fatalf("valid lease renewal rejected: %v", err)
	}
}
