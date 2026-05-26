package claudecode

import (
	"context"
	"sync"
)

// InMemorySessionStore is a SessionStore backed by an in-process map. It is
// intended for development, testing, and single-process use — it does not
// survive a restart and is not shared across hosts. For durable, multi-host
// storage implement SessionStore against S3, Redis, or a database (see
// examples/22_s3_session_store).
type InMemorySessionStore struct {
	mu      sync.Mutex
	entries map[string][]SessionStoreEntry
}

// NewInMemorySessionStore returns an empty InMemorySessionStore.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		entries: make(map[string][]SessionStoreEntry),
	}
}

func memKey(key SessionKey) string {
	k := key.ProjectKey + "/" + key.SessionID
	if key.Subpath != "" {
		k += "/" + key.Subpath
	}
	return k
}

// Append persists a batch of entries for key, preserving order.
func (s *InMemorySessionStore) Append(_ context.Context, key SessionKey, entries []SessionStoreEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := memKey(key)
	for _, e := range entries {
		cp := make(SessionStoreEntry, len(e))
		copy(cp, e)
		s.entries[k] = append(s.entries[k], cp)
	}
	return nil
}

// Load returns the entries previously persisted for key, in order, or
// (nil, nil) when the session is unknown.
func (s *InMemorySessionStore) Load(_ context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.entries[memKey(key)]
	if !ok {
		return nil, nil
	}
	out := make([]SessionStoreEntry, len(stored))
	for i, e := range stored {
		cp := make(SessionStoreEntry, len(e))
		copy(cp, e)
		out[i] = cp
	}
	return out, nil
}
