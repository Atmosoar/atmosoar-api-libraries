package runtimeconfig

import (
	"context"
	"encoding/json"
	"sync"
)

// Store persists config overrides as a key -> raw-JSON map. The forecast
// ingestor's runtime_config table is the reference schema; the pgxstore
// sub-package is the Postgres implementation, MemStore the DB-less one.
type Store interface {
	// LoadAll returns every persisted override. A boot-time error here is
	// non-fatal to the caller (LoadAtBoot proceeds on registry defaults).
	LoadAll(ctx context.Context) (map[string]json.RawMessage, error)
	// Upsert persists a single override (called after Registry.Validate succeeds).
	Upsert(ctx context.Context, key string, value json.RawMessage) error
	// Delete removes an override, reverting the key to its registry default.
	Delete(ctx context.Context, key string) error
	// Kind reports the backing store for the /admin/<svc>/info response
	// ("postgres" or "memory"), so operators see whether overrides survive a
	// restart.
	Kind() string
}

// MemStore is an in-memory Store for services without a SQL database (gateway,
// MMA, elevation). Overrides live for the process lifetime and reset on restart.
type MemStore struct {
	mu sync.RWMutex
	m  map[string]json.RawMessage
}

// NewMemStore returns an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{m: make(map[string]json.RawMessage)}
}

// LoadAll returns a copy of the stored overrides.
func (s *MemStore) LoadAll(_ context.Context) (map[string]json.RawMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]json.RawMessage, len(s.m))
	for k, v := range s.m {
		cp := make(json.RawMessage, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out, nil
}

// Upsert stores a copy of value under key.
func (s *MemStore) Upsert(_ context.Context, key string, value json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(json.RawMessage, len(value))
	copy(cp, value)
	s.m[key] = cp
	return nil
}

// Delete removes key.
func (s *MemStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// Kind reports "memory".
func (s *MemStore) Kind() string { return "memory" }
