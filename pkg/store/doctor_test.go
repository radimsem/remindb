package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func insertNodeRaw(t *testing.T, st *Store, id, parentID string) {
	t.Helper()

	var pid sql.NullString
	if parentID != "" {
		pid = sql.NullString{String: parentID, Valid: true}
	}

	_, err := st.db.Exec(`
		INSERT INTO nodes (id, parent_id, source_file, node_type, depth, label, content, format, token_count, content_hash, temperature)
		VALUES (?, ?, '/x', 'p', 0, 'l', '', 'plain', 0, 'h', 0.5)`,
		id, pid)
	if err != nil {
		t.Fatalf("insert node %s: %v", id, err)
	}
}

func TestCountsOnFreshDB(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		fn   func(context.Context) (int, error)
	}{
		{"CountNodes", st.CountNodes},
		{"CountOrphanParents", st.CountOrphanParents},
		{"CountDanglingDiffs", st.CountDanglingDiffs},
		{"CountSnapshots", st.CountSnapshots},
	} {
		got, err := tc.fn(ctx)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}

		if got != 0 {
			t.Errorf("%s: got %d, want 0", tc.name, got)
		}
	}
}

func TestFTSIntegrityCheckPassesOnFreshDB(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	insertNodeRaw(t, st, "n0000000001", "")
	insertNodeRaw(t, st, "n0000000002", "")

	if err := st.CheckFTSIntegrity(ctx); err != nil {
		t.Fatalf("CheckFTSIntegrity on healthy DB: %v", err)
	}
}

func TestRebuildFTSResolvesIntegrityFailure(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	insertNodeRaw(t, st, "n0000000001", "")
	insertNodeRaw(t, st, "n0000000002", "")

	if _, err := st.db.Exec(`INSERT INTO nodes_fts(nodes_fts) VALUES('delete-all')`); err != nil {
		t.Fatalf("seed delete-all: %v", err)
	}

	if err := st.CheckFTSIntegrity(ctx); err == nil {
		t.Fatalf("expected integrity check to fail after delete-all")
	}

	if err := st.RebuildFTS(ctx); err != nil {
		t.Fatalf("RebuildFTS: %v", err)
	}

	if err := st.CheckFTSIntegrity(ctx); err != nil {
		t.Fatalf("after rebuild: %v", err)
	}

	if err := st.RebuildFTS(ctx); err != nil {
		t.Fatalf("idempotent RebuildFTS: %v", err)
	}
}

func TestPromoteOrphansToRoots(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	if _, err := st.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable FK: %v", err)
	}

	insertNodeRaw(t, st, "n0000000001", "missing9999")
	insertNodeRaw(t, st, "n0000000002", "missing9999")
	insertNodeRaw(t, st, "n0000000003", "")

	if _, err := st.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("re-enable FK: %v", err)
	}

	got, _ := st.CountOrphanParents(ctx)
	if got != 2 {
		t.Fatalf("seeded orphans: got %d, want 2", got)
	}

	if err := st.PromoteOrphansToRoots(ctx); err != nil {
		t.Fatalf("PromoteOrphansToRoots: %v", err)
	}

	got, _ = st.CountOrphanParents(ctx)
	if got != 0 {
		t.Fatalf("after promote: got %d, want 0", got)
	}

	if err := st.PromoteOrphansToRoots(ctx); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
}

func TestDeleteDanglingDiffs(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	if _, err := st.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable FK: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO diffs (snapshot_id, node_id, op) VALUES (9999, 'nX', 'ins')`); err != nil {
		t.Fatalf("seed diff: %v", err)
	}
	if _, err := st.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("re-enable FK: %v", err)
	}

	got, _ := st.CountDanglingDiffs(ctx)
	if got != 1 {
		t.Fatalf("seeded diffs: got %d, want 1", got)
	}

	if err := st.DeleteDanglingDiffs(ctx); err != nil {
		t.Fatalf("DeleteDanglingDiffs: %v", err)
	}

	got, _ = st.CountDanglingDiffs(ctx)
	if got != 0 {
		t.Fatalf("after delete: got %d, want 0", got)
	}

	if err := st.DeleteDanglingDiffs(ctx); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
}

func TestRepointHeadCursorEmptySnapshots(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	if _, err := st.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable FK: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO cursors (id, snapshot_id) VALUES ('HEAD', 9999)`); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
	if _, err := st.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("re-enable FK: %v", err)
	}

	_, exists, _ := st.HeadCursorRef(ctx)
	if !exists {
		t.Fatalf("seed: expected HEAD to exist")
	}

	if err := st.RepointHeadCursor(ctx); err != nil {
		t.Fatalf("RepointHeadCursor: %v", err)
	}

	_, exists, _ = st.HeadCursorRef(ctx)
	if exists {
		t.Fatalf("after repoint with empty snapshots: HEAD should be deleted")
	}

	if err := st.RepointHeadCursor(ctx); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
}

func TestRepointHeadCursorWithSnapshots(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	if _, err := st.db.Exec(`INSERT INTO snapshots (cursor_hash) VALUES ('hash1'), ('hash2'), ('hash3')`); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}
	if _, err := st.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable FK: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO cursors (id, snapshot_id) VALUES ('HEAD', 9999)`); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
	if _, err := st.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("re-enable FK: %v", err)
	}

	if err := st.RepointHeadCursor(ctx); err != nil {
		t.Fatalf("RepointHeadCursor: %v", err)
	}

	snapID, exists, _ := st.HeadCursorRef(ctx)
	if !exists {
		t.Fatalf("HEAD missing after repoint")
	}

	if snapID != 3 {
		t.Fatalf("HEAD snapshot_id: got %d, want 3", snapID)
	}
}

func TestEmbeddedMigrationVersions(t *testing.T) {
	versions, err := EmbeddedMigrationVersions()
	if err != nil {
		t.Fatalf("EmbeddedMigrationVersions: %v", err)
	}

	if len(versions) == 0 {
		t.Fatalf("expected at least one embedded migration")
	}
	for i := 1; i < len(versions); i++ {
		if versions[i] <= versions[i-1] {
			t.Fatalf("not sorted: %v", versions)
		}
	}
}

func TestAppliedMatchesEmbedded(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	applied, err := st.AppliedMigrationVersions(ctx)
	if err != nil {
		t.Fatalf("AppliedMigrationVersions: %v", err)
	}

	embedded, err := EmbeddedMigrationVersions()
	if err != nil {
		t.Fatalf("EmbeddedMigrationVersions: %v", err)
	}

	if len(applied) != len(embedded) {
		t.Fatalf("count mismatch: applied=%d embedded=%d", len(applied), len(embedded))
	}
}

func TestBackupToProducesUsableCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.db")

	st, err := Open(src)
	if err != nil {
		t.Fatalf("Open source: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	insertNodeRaw(t, st, "n0000000001", "")
	insertNodeRaw(t, st, "n0000000002", "")

	dst := filepath.Join(dir, "backup.db")
	if err := st.BackupTo(ctx, dst); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}

	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat backup: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatalf("backup file is empty")
	}

	backup, err := Open(dst)
	if err != nil {
		t.Fatalf("Open backup: %v", err)
	}
	t.Cleanup(func() { _ = backup.Close() })

	got, err := backup.CountNodes(ctx)
	if err != nil {
		t.Fatalf("CountNodes on backup: %v", err)
	}
	if got != 2 {
		t.Fatalf("backup nodes: got %d, want 2", got)
	}
}
