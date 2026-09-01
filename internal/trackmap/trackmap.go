// Package trackmap provides a small mutex-guarded map for the CI trackers.
//
// Each platform tracker keeps the same three lookups — the CI unit's current state,
// its display handle, and its spinner — keyed by whatever identifies a unit on that
// platform: a job or check ID for GitLab and GitHub, a status-context name for
// Forgejo. The bookkeeping around those maps was written out three times, once per
// platform, differing only in the key and value types.
//
// Only that bookkeeping lives here. How a job transitions from pending to running,
// and what its label should read, stay in each platform's tracker: GitLab jobs,
// GitHub checks and Forgejo commit statuses genuinely differ, and folding them
// together would trade a small saving for an abstraction that hides real behaviour.
package trackmap

import "sync"

// Map is a map guarded by a read-write mutex, safe for concurrent use.
//
// Each method locks for its own call only. Callers that read a value, decide
// something from it, and write it back are not doing so atomically — that matches
// how the trackers have always behaved, since a spinner refresh runs concurrently
// with the poll loop and both merely observe the latest state.
type Map[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// New returns an empty Map ready for use.
func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{m: make(map[K]V)}
}

// Get returns the value stored for key, and whether it was present.
//
//nolint:ireturn // V is the container's element type; returning it is the point.
func (t *Map[K, V]) Get(key K) (V, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	v, ok := t.m[key]
	return v, ok
}

// Set stores value under key, replacing any previous entry.
func (t *Map[K, V]) Set(key K, value V) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.m[key] = value
}

// Delete removes key. Deleting an absent key is a no-op.
func (t *Map[K, V]) Delete(key K) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.m, key)
}

// Keys returns the keys currently present, in unspecified order.
//
// The slice is a snapshot: callers can iterate it while other goroutines mutate the
// map, which is what the trackers need when comparing the previous poll's units
// against the current one.
func (t *Map[K, V]) Keys() []K {
	t.mu.RLock()
	defer t.mu.RUnlock()

	keys := make([]K, 0, len(t.m))
	for k := range t.m {
		keys = append(keys, k)
	}
	return keys
}

// Len reports how many entries the map holds.
func (t *Map[K, V]) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.m)
}
