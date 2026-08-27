package trackmap_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/sgaunet/auto-mr/internal/trackmap"
)

func TestGetSetDelete(t *testing.T) {
	m := trackmap.New[int64, string]()

	if _, ok := m.Get(1); ok {
		t.Error("Get reported a value for an empty map")
	}

	m.Set(1, "one")
	got, ok := m.Get(1)
	if !ok || got != "one" {
		t.Errorf("Get(1) = %q, %v; want \"one\", true", got, ok)
	}

	// Setting an existing key replaces rather than duplicates.
	m.Set(1, "uno")
	if got, _ := m.Get(1); got != "uno" {
		t.Errorf("Get(1) after overwrite = %q, want \"uno\"", got)
	}
	if m.Len() != 1 {
		t.Errorf("Len = %d after overwriting one key, want 1", m.Len())
	}

	m.Delete(1)
	if _, ok := m.Get(1); ok {
		t.Error("Get reported a value after Delete")
	}
	// Deleting an absent key must be harmless: the trackers delete spinners that may
	// already have been finalised.
	m.Delete(1)
}

func TestKeysSnapshot(t *testing.T) {
	m := trackmap.New[string, int]()
	for _, k := range []string{"a", "b", "c"} {
		m.Set(k, 1)
	}

	keys := m.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys returned %d entries, want 3", len(keys))
	}

	// The result is a snapshot, so mutating the map afterwards must not change it.
	m.Delete("a")
	if len(keys) != 3 {
		t.Errorf("Keys result changed after a later Delete; it is not a snapshot")
	}
}

func TestKeysOnEmptyMap(t *testing.T) {
	if keys := trackmap.New[int, int]().Keys(); len(keys) != 0 {
		t.Errorf("Keys on an empty map returned %v", keys)
	}
}

// TestConcurrentAccess exercises the property the type exists for: the poll loop
// writes state while spinner goroutines read it. Run under -race this is the real
// assertion; the counts merely confirm the work happened.
func TestConcurrentAccess(t *testing.T) {
	m := trackmap.New[string, int]()

	const workers = 8
	const perWorker = 200

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range perWorker {
				key := strconv.Itoa(w) + "-" + strconv.Itoa(i)
				m.Set(key, i)
				m.Get(key)
				m.Keys()
				if i%2 == 0 {
					m.Delete(key)
				}
			}
		}(w)
	}
	wg.Wait()

	// Half of each worker's keys were deleted.
	if want := workers * perWorker / 2; m.Len() != want {
		t.Errorf("Len = %d, want %d", m.Len(), want)
	}
}
