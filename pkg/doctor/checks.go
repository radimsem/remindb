package doctor

import (
	"context"
	"fmt"
	"os"

	"github.com/radimsem/remindb/pkg/store"
)

// AllChecks returns the canonical, ordered set of doctor checks.
func AllChecks() []Check {
	return []Check{
		ftsSyncCheck(),
		orphanParentCheck(),
		headCursorCheck(),
		danglingDiffsCheck(),
		schemaVersionCheck(),
		staleCompileRootCheck(),
	}
}

func ftsSyncCheck() Check {
	return Check{
		Name: "fts5_sync",
		Run: func(ctx context.Context, st *store.Store) Result {
			if err := st.CheckFTSIntegrity(ctx); err != nil {
				return Result{Status: Fail, Detail: "FTS5 integrity check failed: " + err.Error()}
			}

			n, err := st.CountNodes(ctx)
			if err != nil {
				return Result{Status: Fail, Detail: "failed to count nodes: " + err.Error()}
			}

			return Result{Status: Pass, Detail: fmt.Sprintf("FTS5 index in sync with %d nodes", n)}
		},
		Fix: func(ctx context.Context, st *store.Store) error { return st.RebuildFTS(ctx) },
	}
}

func orphanParentCheck() Check {
	return Check{
		Name: "orphan_parent_id",
		Run: func(ctx context.Context, st *store.Store) Result {
			n, err := st.CountOrphanParents(ctx)
			if err != nil {
				return Result{Status: Fail, Detail: "failed to count orphans: " + err.Error()}
			}

			if n == 0 {
				return Result{Status: Pass, Detail: "no orphans"}
			}
			return Result{Status: Fail, Detail: fmt.Sprintf("%d nodes reference a missing parent_id", n)}
		},
		Fix: func(ctx context.Context, st *store.Store) error { return st.PromoteOrphansToRoots(ctx) },
	}
}

func headCursorCheck() Check {
	return Check{
		Name: "head_cursor",
		Run: func(ctx context.Context, st *store.Store) Result {
			snapID, hasHead, err := st.HeadCursorRef(ctx)
			if err != nil {
				return Result{Status: Fail, Detail: "failed to read HEAD cursor: " + err.Error()}
			}

			if !hasHead {
				return Result{Status: Pass, Detail: "no HEAD cursor"}
			}

			exists, err := st.SnapshotExists(ctx, snapID)
			if err != nil {
				return Result{Status: Fail, Detail: "failed to verify HEAD snapshot: " + err.Error()}
			}
			if !exists {
				return Result{Status: Fail, Detail: fmt.Sprintf("HEAD points at missing snapshot %d", snapID)}
			}

			return Result{Status: Pass, Detail: fmt.Sprintf("HEAD → snapshot %d", snapID)}
		},
		Fix: func(ctx context.Context, st *store.Store) error { return st.RepointHeadCursor(ctx) },
	}
}

func danglingDiffsCheck() Check {
	return Check{
		Name: "dangling_diffs",
		Run: func(ctx context.Context, st *store.Store) Result {
			n, err := st.CountDanglingDiffs(ctx)
			if err != nil {
				return Result{Status: Fail, Detail: "failed to count dangling diffs: " + err.Error()}
			}

			if n == 0 {
				return Result{Status: Pass, Detail: "no dangling diffs"}
			}
			return Result{Status: Fail, Detail: fmt.Sprintf("%d diff rows reference a missing snapshot", n)}
		},
		Fix: func(ctx context.Context, st *store.Store) error { return st.DeleteDanglingDiffs(ctx) },
	}
}

func schemaVersionCheck() Check {
	return Check{
		Name: "schema_version",
		Run: func(ctx context.Context, st *store.Store) Result {
			applied, err := st.AppliedMigrationVersions(ctx)
			if err != nil {
				return Result{Status: Fail, Detail: "failed to read schema_migrations: " + err.Error()}
			}

			embedded, err := store.EmbeddedMigrationVersions()
			if err != nil {
				return Result{Status: Fail, Detail: "failed to read embedded migrations: " + err.Error()}
			}

			missing := diffSorted(embedded, applied)
			if len(missing) == 0 {
				return Result{Status: Pass, Detail: fmt.Sprintf("%d/%d migrations applied", len(applied), len(embedded))}
			}

			return Result{Status: Fail, Detail: fmt.Sprintf("missing: %v (applied=%d, embedded=%d)", missing, len(applied), len(embedded))}
		},
		Fix: func(ctx context.Context, st *store.Store) error { return st.Migrate(ctx) },
	}
}

func staleCompileRootCheck() Check {
	return Check{
		Name: "stale_compile_root",
		Run: func(ctx context.Context, st *store.Store) Result {
			roots, err := st.DistinctCompileRoots(ctx)
			if err != nil {
				return Result{Status: Fail, Detail: "failed to list compile roots: " + err.Error()}
			}

			var missing []string
			for _, r := range roots {
				if _, err := os.Stat(r); os.IsNotExist(err) {
					missing = append(missing, r)
				}
			}

			if len(missing) == 0 {
				return Result{Status: Pass, Detail: fmt.Sprintf("%d compile roots present", len(roots))}
			}
			return Result{Status: Warn, Detail: fmt.Sprintf("%d/%d compile roots no longer exist: %v", len(missing), len(roots), missing)}
		},
	}
}

// diffSorted returns elements present in want but absent from have. Both inputs must be sorted.
func diffSorted(want, have []string) []string {
	got := make(map[string]struct{}, len(have))
	for _, v := range have {
		got[v] = struct{}{}
	}

	var out []string
	for _, v := range want {
		if _, ok := got[v]; !ok {
			out = append(out, v)
		}
	}
	return out
}
