package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// Simulate two independent clients writing disjoint nodes in parallel.
func TestConcurrent_TwoWritersSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")

	a := openShared(t, path, true)
	b := openShared(t, path, false)

	const perWriter = 25
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)

	errs := make(chan error, 2*perWriter)

	go func() {
		defer wg.Done()
		for i := 0; i < perWriter; i++ {
			id := fmt.Sprintf("a_%06d", i)
			if err := a.UpsertNode(ctx, testNode(id, "")); err != nil {
				errs <- fmt.Errorf("a: %w", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < perWriter; i++ {
			id := fmt.Sprintf("b_%06d", i)
			if err := b.UpsertNode(ctx, testNode(id, "")); err != nil {
				errs <- fmt.Errorf("b: %w", err)
				return
			}
		}
	}()

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent write: %v", err)
	}

	for i := 0; i < perWriter; i++ {
		for _, prefix := range []string{"a_", "b_"} {
			id := fmt.Sprintf("%s%06d", prefix, i)

			if _, err := a.GetNode(ctx, id); err != nil {
				t.Errorf("GetNode %s (via a): %v", id, err)
			}
			if _, err := b.GetNode(ctx, id); err != nil {
				t.Errorf("GetNode %s (via b): %v", id, err)
			}
		}
	}
}

// Verify that parallel snapshot creation across two handles still produces unique IDs.
func TestConcurrent_SnapshotIDsMonotonic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")

	a := openShared(t, path, true)
	b := openShared(t, path, false)

	ctx := context.Background()
	const per = 15

	var wg sync.WaitGroup
	wg.Add(2)

	for idx, st := range []*Store{a, b} {
		go func() {
			defer wg.Done()

			for i := 0; i < per; i++ {
				hash := fmt.Sprintf("h%d_%013d", idx, i)
				err := st.Tx(ctx, func(tx *sql.Tx) error {
					id, err := st.CreateSnapshotTx(ctx, tx, hash, "concurrent", "")
					if err != nil {
						return err
					}

					return st.AdvanceCursorTx(ctx, tx, id)
				})

				if err != nil {
					t.Errorf("writer %d snapshot Tx: %v", idx, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	snaps, err := a.ListSnapshots(ctx, 2*per+5)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 2*per {
		t.Fatalf("snapshots = %d, want %d", len(snaps), 2*per)
	}

	ids := make(map[int64]bool, len(snaps))
	for _, s := range snaps {
		if ids[s.ID] {
			t.Errorf("duplicate snapshot id %d", s.ID)
		}

		ids[s.ID] = true
	}
}

func openShared(t *testing.T, path string, migrate bool) *Store {
	t.Helper()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if migrate {
		if err := st.Migrate(context.Background()); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
	}
	return st
}
