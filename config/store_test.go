package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuleStore_Reload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")

	initial := []byte(`tiers:
  free:
    capacity: 100
    refill_rate: 10
ips:
  capacity: 500
  refill_rate: 50
endpoints:
  /api/test:
    rule: endpoint
    cost: 1
    global_capacity: 1000
    global_refill_rate: 100
`)
	if err := os.WriteFile(path, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewRuleStore(path)
	if err != nil {
		t.Fatalf("NewRuleStore: %v", err)
	}
	if store.Get().Tiers["free"].Capacity != 100 {
		t.Fatalf("expected initial capacity 100, got %d", store.Get().Tiers["free"].Capacity)
	}

	updated := []byte(`tiers:
  free:
    capacity: 200
    refill_rate: 20
ips:
  capacity: 500
  refill_rate: 50
endpoints:
  /api/test:
    rule: endpoint
    cost: 1
    global_capacity: 1000
    global_refill_rate: 100
`)
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if store.Get().Tiers["free"].Capacity != 200 {
		t.Fatalf("expected reloaded capacity 200, got %d", store.Get().Tiers["free"].Capacity)
	}
}

func TestRuleStore_ReloadKeepsPreviousOnInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")

	valid := []byte(`tiers:
  free:
    capacity: 100
    refill_rate: 10
ips:
  capacity: 500
  refill_rate: 50
endpoints:
  /api/test:
    rule: endpoint
    cost: 1
    global_capacity: 1000
    global_refill_rate: 100
`)
	if err := os.WriteFile(path, valid, 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewRuleStore(path)
	if err != nil {
		t.Fatalf("NewRuleStore: %v", err)
	}

	invalid := []byte(`tiers:
  free:
    capacity: -1
    refill_rate: 10
ips:
  capacity: 500
  refill_rate: 50
endpoints:
  /api/test:
    rule: endpoint
    cost: 1
    global_capacity: 1000
    global_refill_rate: 100
`)
	if err := os.WriteFile(path, invalid, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Reload(); err == nil {
		t.Fatal("expected reload error for invalid config")
	}
	if store.Get().Tiers["free"].Capacity != 100 {
		t.Fatalf("expected previous config kept, got capacity %d", store.Get().Tiers["free"].Capacity)
	}
}

func TestRuleStore_WatchReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")

	initial := []byte(`tiers:
  free:
    capacity: 100
    refill_rate: 10
ips:
  capacity: 500
  refill_rate: 50
endpoints:
  /api/test:
    rule: endpoint
    cost: 1
    global_capacity: 1000
    global_refill_rate: 100
`)
	if err := os.WriteFile(path, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewRuleStore(path)
	if err != nil {
		t.Fatalf("NewRuleStore: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Watch(ctx)

	updated := []byte(`tiers:
  free:
    capacity: 300
    refill_rate: 30
ips:
  capacity: 500
  refill_rate: 50
endpoints:
  /api/test:
    rule: endpoint
    cost: 1
    global_capacity: 1000
    global_refill_rate: 100
`)
	// Simulate atomic editor save (write temp + rename).
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if store.Get().Tiers["free"].Capacity == 300 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("watch did not reload config within timeout, capacity=%d", store.Get().Tiers["free"].Capacity)
}
