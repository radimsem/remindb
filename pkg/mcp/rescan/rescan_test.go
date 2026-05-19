package rescan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/mcp/rescanlog"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
	"github.com/radimsem/remindb/pkg/store"
)

func mustRescan(t *testing.T, st *store.Store, dir string, interval time.Duration, logger *slog.Logger) *Loop {
	t.Helper()

	r, err := New(st, dir, interval, WithLogger(logger))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return r
}

func TestRescanLoop_LogsWalkErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ok.md", "# OK\n")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, logger)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	r.walkFn = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "nope"), nil, errors.New("forced walk error"))
	}
	r.scan(context.Background())

	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("expected WARN log for walk error, got: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "nope") {
		t.Errorf("expected path %q in log, got: %q", "nope", buf.String())
	}
}

func TestRescanLoop_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.md", "# Notes\n")

	hidden := filepath.Join(dir, ".obsidian")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, hidden, "prefs.json", `{"hidden": true}`)

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }
	r.scan(context.Background())

	for path := range r.modTimes {
		if filepath.Base(filepath.Dir(path)) == ".obsidian" {
			t.Errorf("indexed hidden path: %s", path)
		}
	}
	if len(r.modTimes) != 1 {
		t.Errorf("mtimes = %d, want 1 (notes.md only)", len(r.modTimes))
	}
}

func TestRescanLoop_DetectsChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Original\n\nContent.\n")

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()

	r.scan(ctx)
	nodes, _ := st.GetAllNodes(ctx)
	for _, n := range nodes {
		if strings.Contains(n.Content, "Updated") {
			t.Fatalf("unexpected updated content before edit: %q", n.Content)
		}
	}

	time.Sleep(10 * time.Millisecond)
	writeFile(t, dir, "doc.md", "# Updated\n\nNew content.\n")

	r.scan(ctx)
	nodes, _ = st.GetAllNodes(ctx)

	foundUpdated := false
	for _, n := range nodes {
		if strings.Contains(n.Content, "New content") {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Error("expected updated content after recompile")
	}
}

func TestRescanLoop_DebouncesMidSave(t *testing.T) {
	dir := t.TempDir()

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)

	frozen := time.Now()
	r.now = func() time.Time { return frozen }
	writeFile(t, dir, "doc.md", "# Fresh\n\nContent.\n")

	ctx := context.Background()
	r.scan(ctx)
	roots, _ := st.GetRootNodes(ctx)
	if len(roots) != 0 {
		t.Errorf("expected no nodes — file is still settling, got %d", len(roots))
	}

	r.now = func() time.Time { return frozen.Add(r.settle + time.Second) }
	r.scan(ctx)
	roots, _ = st.GetRootNodes(ctx)
	if len(roots) == 0 {
		t.Error("expected nodes after file has settled")
	}
}

func TestRescanLoop_CommitsMtimesOnlyAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ok.md", "# OK\n")
	writeFile(t, dir, "bad.json", `{"unterminated`)

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()
	r.scan(ctx)

	if len(r.modTimes) != 0 {
		t.Errorf("mtimes = %d, want 0 (compile failed, nothing committed)", len(r.modTimes))
	}

	writeFile(t, dir, "bad.json", `{"valid": "now"}`)

	r.scan(ctx)

	if len(r.modTimes) != 2 {
		t.Errorf("mtimes = %d, want 2 (both files compiled on retry)", len(r.modTimes))
	}
}

func TestRescanLoop_ReconcilesDeletedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.md", "# Keep\n")
	writeFile(t, dir, "gone.md", "# Gone\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()

	r.scan(ctx)

	before, err := st.GetNodesByFile(ctx, "gone.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("expected gone.md nodes after initial scan")
	}

	if err := os.Remove(filepath.Join(dir, "gone.md")); err != nil {
		t.Fatal(err)
	}

	r.scan(ctx)

	after, err := st.GetNodesByFile(ctx, "gone.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("orphan nodes after delete = %d, want 0", len(after))
	}

	kept, err := st.GetNodesByFile(ctx, "keep.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(kept) == 0 {
		t.Error("keep.md nodes should remain")
	}
}

func TestRescanLoop_PublishesStatus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.md", "# Keep\n")
	writeFile(t, dir, "gone.md", "# Gone\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	status := rescanstat.New()

	r, err := New(st, dir, 90*time.Second, WithStatus(status))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()
	r.scan(ctx)

	iv, snap := status.Get()
	if iv != 90 {
		t.Errorf("interval_s = %d, want 90", iv)
	}

	if snap.RunAt == 0 {
		t.Error("run_at should be set after a scan")
	}
	if snap.Error != "" {
		t.Errorf("error = %q, want empty after a clean scan", snap.Error)
	}
	if snap.Added == 0 {
		t.Errorf("added = %d, want > 0 after the initial compile", snap.Added)
	}
	if len(snap.PurgedFiles) != 0 {
		t.Errorf("purged_files = %v, want none before any deletion", snap.PurgedFiles)
	}

	goneNodes, err := st.GetNodesByFile(ctx, "gone.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(goneNodes) == 0 {
		t.Fatal("expected gone.md nodes after initial scan")
	}

	if err := os.Remove(filepath.Join(dir, "gone.md")); err != nil {
		t.Fatal(err)
	}
	r.scan(ctx)

	_, snap = status.Get()
	if len(snap.PurgedFiles) != 1 {
		t.Fatalf("purged_files = %v, want exactly one entry", snap.PurgedFiles)
	}

	pf := snap.PurgedFiles[0]
	if pf.Path != "gone.md" {
		t.Errorf("purged path = %q, want %q", pf.Path, "gone.md")
	}
	if pf.Nodes != len(goneNodes) {
		t.Errorf("purged nodes = %d, want %d", pf.Nodes, len(goneNodes))
	}
}

func TestRescanLoop_PersistsTickAndExcludesFromWalk(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Doc\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	sink, err := rescanlog.New(dir, 1<<20)
	if err != nil {
		t.Fatalf("rescanlog.New: %v", err)
	}

	r, err := New(st, dir, 45*time.Second, WithRescanLog(sink))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()
	r.scan(ctx) // tick 1: compiles doc.md and writes rescan.jsonl
	r.scan(ctx) // tick 2: rescan.jsonl is now on disk while the tree is walked

	data, err := os.ReadFile(rescanlog.Path(dir))
	if err != nil {
		t.Fatalf("read rescan.jsonl: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("rescan.jsonl line count = %d, want 2 (one per tick)", len(lines))
	}

	for i, l := range lines {
		var snap rescanstat.Snapshot
		if err := json.Unmarshal([]byte(l), &snap); err != nil {
			t.Fatalf("tick %d line not valid Snapshot JSON: %v", i, err)
		}

		if snap.RunAt == 0 {
			t.Errorf("tick %d run_at unset", i)
		}
	}

	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}

	if len(nodes) == 0 {
		t.Fatal("expected doc.md nodes after scan")
	}
	for _, n := range nodes {
		if strings.Contains(filepath.ToSlash(n.SourceFile), config.DirName+"/") {
			t.Errorf("indexed file inside %s/: %s", config.DirName, n.SourceFile)
		}
	}
}

func TestRescanLoop_RecordsDeletionsInSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.md", "# Keep\n")
	writeFile(t, dir, "gone.md", "# Gone\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()
	r.scan(ctx)

	goneNodes, err := st.GetNodesByFile(ctx, "gone.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(goneNodes) == 0 {
		t.Fatal("expected gone.md nodes after initial scan")
	}

	snapsBefore, _ := st.ListSnapshots(ctx, 10)

	if err := os.Remove(filepath.Join(dir, "gone.md")); err != nil {
		t.Fatal(err)
	}
	r.scan(ctx)

	snapsAfter, _ := st.ListSnapshots(ctx, 10)
	if len(snapsAfter) != len(snapsBefore)+1 {
		t.Fatalf("snapshots = %d, want %d (one new purge snapshot)", len(snapsAfter), len(snapsBefore)+1)
	}

	purge := snapsAfter[0]
	if !strings.Contains(purge.Message, "purged") {
		t.Errorf("purge snapshot message = %q, want to contain %q", purge.Message, "purged")
	}

	diffs, err := st.GetDiffsBySnapshot(ctx, purge.ID)
	if err != nil {
		t.Fatalf("GetDiffsBySnapshot: %v", err)
	}
	if len(diffs) != len(goneNodes) {
		t.Errorf("diff records = %d, want %d (one per deleted node)", len(diffs), len(goneNodes))
	}

	for _, d := range diffs {
		if d.Op != "rem" {
			t.Errorf("diff op = %q, want %q", d.Op, "rem")
		}
		if d.OldHash == "" || d.OldContent == "" {
			t.Errorf("diff missing old hash/content: %+v", d)
		}
	}

	cursor, _ := st.GetHeadCursorHash(ctx)
	if cursor != purge.CursorHash {
		t.Errorf("HEAD cursor = %q, want %q", cursor, purge.CursorHash)
	}
}

func TestRescanLoop_SkipsPurgeOnWalkError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.md", "# Keep\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()
	r.scan(ctx)

	before, err := st.GetNodesByFile(ctx, "keep.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("expected keep.md nodes after initial scan")
	}

	snapsBefore, _ := st.ListSnapshots(ctx, 10)

	r.walkFn = func(root string, fn fs.WalkDirFunc) error {
		return errors.New("forced walk failure")
	}

	r.scan(ctx)

	after, err := st.GetNodesByFile(ctx, "keep.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("keep.md nodes = %d, want %d (walk failed, must not purge)", len(after), len(before))
	}

	snapsAfter, _ := st.ListSnapshots(ctx, 10)
	if len(snapsAfter) != len(snapsBefore) {
		t.Errorf("snapshots changed: before=%d after=%d (walk failed, must not emit purge snapshot)",
			len(snapsBefore), len(snapsAfter))
	}

	if _, ok := r.modTimes[filepath.Join(dir, "keep.md")]; !ok {
		t.Error("modTimes entry dropped despite walk error")
	}
}

func TestRescanLoop_NewFile(t *testing.T) {
	dir := t.TempDir()

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()
	r.scan(ctx)

	writeFile(t, dir, "new.md", "# New\n\nContent.\n")

	r.scan(ctx)

	roots, _ := st.GetRootNodes(ctx)
	if len(roots) == 0 {
		t.Error("expected nodes from new file")
	}
}

func TestRescanLoop_ScanBlocksOnOpMu(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Doc\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	st.OpMu.Lock()

	done := make(chan struct{})
	go func() {
		r.scan(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("scan completed while OpMu was held")
	case <-time.After(50 * time.Millisecond):
	}

	st.OpMu.Unlock()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("scan did not complete after OpMu.Unlock")
	}
}

func TestRescanLoop_RunCatchesStaleEditsAtStartup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Before\n\nOriginal body.\n")

	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	if _, err := compiler.CompileDir(ctx, st, dir, "initial"); err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	writeFile(t, dir, "doc.md", "# Before\n\nUpdated body.\n")

	r := mustRescan(t, st, dir, time.Hour, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	r.Run(runCtx)

	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}

	foundUpdated := false
	for _, n := range nodes {
		if strings.Contains(n.Content, "Original body") {
			t.Errorf("stale content persisted after startup reconcile: %q", n.Content)
		}
		if strings.Contains(n.Content, "Updated body") {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Error("startup reconcile did not apply edit made while serve was down")
	}
}

func TestRescanLoop_RespectsIgnore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "kept.md", "# Kept\n")
	writeFile(t, dir, "session.jsonl", `{"event":"chat"}`)
	writeFile(t, dir, ignore.Path, "*.jsonl\n")

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	r.scan(context.Background())

	for path := range r.modTimes {
		if strings.HasSuffix(path, ".jsonl") {
			t.Errorf("rescan tracked excluded jsonl file: %s", path)
		}
	}
	if len(r.modTimes) != 1 {
		t.Errorf("modTimes = %d, want 1 (kept.md only)", len(r.modTimes))
	}
}

func TestNewRescanLoop_FailsOnMalformedIgnore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ignore.Path, "a//b\n")

	st := testutil.OpenTestDB(t)
	_, err := New(st, dir, time.Minute)
	if err == nil {
		t.Fatal("expected error for malformed ignore file")
	}
	if !strings.Contains(err.Error(), ignore.Path) {
		t.Errorf("error should mention %s, got: %v", ignore.Path, err)
	}
}

func TestRescanLoop_ReloadDefaultsWhenConfigAbsent(t *testing.T) {
	dir := t.TempDir()
	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, 42*time.Second, nil)

	if changed := r.reloadConfig(); changed {
		t.Error("interval should not change vs bootstrap when no config block present")
	}
	if !r.enabled {
		t.Error("enabled should default to true when block absent")
	}

	if r.interval != 42*time.Second {
		t.Errorf("interval = %v, want bootstrap 42s", r.interval)
	}
	if r.settle != defaultSettleTime {
		t.Errorf("settle = %v, want default %v", r.settle, defaultSettleTime)
	}
}

func TestRescanLoop_ReloadAppliesBlock(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, config.Path, `{"rescan":{"enabled":false,"interval":"5s","settle":"1s"}}`)

	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, 30*time.Second, nil)

	if !r.reloadConfig() {
		t.Error("interval changed 30s→5s, want intervalChanged=true")
	}
	if r.enabled {
		t.Error("enabled should be false from config")
	}

	if r.interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s", r.interval)
	}
	if r.settle != time.Second {
		t.Errorf("settle = %v, want 1s", r.settle)
	}

	if r.reloadConfig() {
		t.Error("re-reading identical config must report no interval change")
	}
}

func TestRescanLoop_InvalidReloadKeepsLastGood(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, config.Path, `{"rescan":{"enabled":false,"interval":"5s"}}`)

	st := testutil.OpenTestDB(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := mustRescan(t, st, dir, 30*time.Second, logger)

	r.reloadConfig()
	if r.enabled || r.interval != 5*time.Second {
		t.Fatalf("precondition: good config not applied (enabled=%v interval=%v)", r.enabled, r.interval)
	}

	writeFile(t, dir, config.Path, `{"rescan":{"interval":"-3s"}}`)
	if r.reloadConfig() {
		t.Error("invalid reload must not report an interval change")
	}
	if r.enabled || r.interval != 5*time.Second {
		t.Errorf("last-good not retained: enabled=%v interval=%v", r.enabled, r.interval)
	}
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("expected WARN on invalid reload, got %q", buf.String())
	}

	writeFile(t, dir, config.Path, `{"rescan":{"interval":"7s"}}`)
	r.reloadConfig()
	if r.interval != 7*time.Second {
		t.Errorf("recovery after invalid reload: interval = %v, want 7s", r.interval)
	}
}

func TestRescanLoop_DisabledTickIsNoopThenResumes(t *testing.T) {
	dir := t.TempDir()
	st := testutil.OpenTestDB(t)
	r := mustRescan(t, st, dir, 30*time.Second, nil)

	var walks atomic.Int32
	r.walkFn = func(root string, fn fs.WalkDirFunc) error {
		walks.Add(1)
		return nil
	}

	writeFile(t, dir, config.Path, `{"rescan":{"enabled":false,"interval":"20ms"}}`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond) // many 20ms ticks while disabled
	if n := walks.Load(); n != 0 {
		t.Fatalf("disabled loop performed %d walks, want 0", n)
	}

	writeFile(t, dir, config.Path, `{"rescan":{"enabled":true,"interval":"20ms"}}`)
	time.Sleep(150 * time.Millisecond) // a later tick reloads → enabled → scans

	if walks.Load() == 0 {
		t.Error("re-enabling did not resume scanning on a subsequent tick (no restart expected)")
	}

	cancel()
	<-done
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
