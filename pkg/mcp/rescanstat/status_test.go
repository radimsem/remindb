package rescanstat

import (
	"sync"
	"testing"
)

func TestStatus_SetGet(t *testing.T) {
	s := New()

	iv, snap := s.Get()
	if iv != 0 || snap.RunAt != 0 || snap.PurgedFiles != nil {
		t.Fatalf("zero Status should be empty, got interval=%d snap=%+v", iv, snap)
	}

	want := Snapshot{
		RunAt:       42,
		Added:       1,
		Modified:    2,
		Removed:     3,
		PurgedFiles: []PurgedFile{{Path: "a.md", Nodes: 4}},
	}
	s.Set(30, want)

	iv, got := s.Get()
	if iv != 30 {
		t.Errorf("interval = %d, want 30", iv)
	}
	if got.RunAt != 42 || got.Added != 1 || got.Modified != 2 || got.Removed != 3 {
		t.Errorf("snapshot counts mismatch: %+v", got)
	}
	if len(got.PurgedFiles) != 1 || got.PurgedFiles[0] != (PurgedFile{Path: "a.md", Nodes: 4}) {
		t.Errorf("purged files mismatch: %+v", got.PurgedFiles)
	}
}

func TestStatus_ConcurrentSetGet(t *testing.T) {
	s := New()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func() { defer wg.Done(); s.Set(int64(i), Snapshot{RunAt: int64(i)}) }()
		go func() { defer wg.Done(); s.Get() }()
	}
	wg.Wait()

	if iv, _ := s.Get(); iv < 0 || iv > 49 {
		t.Errorf("interval out of expected range: %d", iv)
	}
}
