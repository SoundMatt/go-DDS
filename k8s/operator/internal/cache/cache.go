// Package cache holds the operator's in-memory view of DDSParticipant and
// DDSDomain custom resources (and the DDSDomain-per-namespace binding),
// kept warm by dynamic informers in internal/controller and read by the
// admission webhook on every pod create/update — the webhook must answer
// synchronously within the API server's timeout, so it never makes a live
// API call itself.
package cache

import "sync"

// Store is a minimal thread-safe key/value cache. Keys are
// "namespace/name" strings (see Key) unless documented otherwise by the
// specific store built on top of it.
type Store[T any] struct {
	mu   sync.RWMutex
	data map[string]T
}

// NewStore returns an empty Store.
func NewStore[T any]() *Store[T] {
	return &Store[T]{data: make(map[string]T)}
}

// Get returns the value for key and whether it was present.
func (s *Store[T]) Get(key string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// Set stores v under key.
func (s *Store[T]) Set(key string, v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = v
}

// Delete removes key, if present.
func (s *Store[T]) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// Len returns the number of entries currently cached (used by tests and by
// the controller's readiness/debug logging).
func (s *Store[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

// Key builds the "namespace/name" cache key used by the participant and
// domain stores.
func Key(namespace, name string) string {
	return namespace + "/" + name
}
