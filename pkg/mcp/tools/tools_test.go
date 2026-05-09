package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

func setup(t *testing.T) (*Deps, *store.Store) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	tracker, err := temperature.NewTracker(st, temperature.DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	d := &Deps{
		Store:   st,
		Engine:  query.NewEngine(st),
		Tracker: tracker,
	}
	return d, st
}

func TestHandleFetch(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	n := &store.Node{
		ID: "testnode", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "test", Content: "hello world",
		Format: "plain", TokenCount: 10, ContentHash: "abc123",
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	result, _, err := d.HandleFetch(ctx, &gomcp.CallToolRequest{}, FetchInput{
		Anchor: "testnode", Budget: 1000,
	})
	if err != nil {
		t.Fatalf("HandleFetch: %v", err)
	}
	if len(result.Content) == 0 {
		t.Error("empty content")
	}
}

func TestHandleSearch(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	n := &store.Node{
		ID: "testnode", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "fox story", Content: "the quick brown fox",
		Format: "plain", TokenCount: 10, ContentHash: "abc123",
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	result, _, err := d.HandleSearch(ctx, &gomcp.CallToolRequest{}, SearchInput{
		Query: "fox", Budget: 1000,
	})
	if err != nil {
		t.Fatalf("HandleSearch: %v", err)
	}
	if len(result.Content) == 0 {
		t.Error("empty content")
	}
}

func TestHandleWrite(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	result, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{
		Payload: "some new memory content",
	})
	if err != nil {
		t.Fatalf("HandleWrite: %v", err)
	}
	if len(result.Content) == 0 {
		t.Error("empty content")
	}
}

func TestHandleWrite_Update(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	// Simulate a node originally compiled from a file.
	n := &store.Node{
		ID: "anchor01", SourceFile: "docs/arch.md", NodeType: "heading",
		Depth: 2, Label: "old", Content: "old content",
		Format: "toon", TokenCount: 10, ContentHash: "old_hash",
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	_, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{
		Anchor: "anchor01", Payload: "updated content",
	})
	if err != nil {
		t.Fatalf("HandleWrite: %v", err)
	}

	got, _ := st.GetNode(ctx, "anchor01")
	if got.Content != "updated content" {
		t.Errorf("Content = %q, want 'updated content'", got.Content)
	}
	if got.SourceFile != "docs/arch.md" {
		t.Errorf("SourceFile = %q, want preserved 'docs/arch.md'", got.SourceFile)
	}
	if got.NodeType != "heading" {
		t.Errorf("NodeType = %q, want preserved 'heading'", got.NodeType)
	}
	if got.Format != "toon" {
		t.Errorf("Format = %q, want preserved 'toon'", got.Format)
	}
}

func TestHandleCompile(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(p, []byte("# Test\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, _, err := d.HandleCompile(ctx, &gomcp.CallToolRequest{}, CompileInput{
		Path: dir, Message: "test compile",
	})
	if err != nil {
		t.Fatalf("HandleCompile: %v", err)
	}
	if len(result.Content) == 0 {
		t.Error("empty content")
	}
}

func TestHandleCompile_AnchorsToSourceDir(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(p, []byte("# Test\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := compiler.CompileDir(ctx, st, dir, "initial"); err != nil {
		t.Fatalf("initial CompileDir: %v", err)
	}

	before, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats before: %v", err)
	}
	if before.NodeCount == 0 {
		t.Fatal("initial compile produced 0 nodes")
	}

	d.SourceDir = dir
	// Concat (not filepath.Join) to keep the non-canonical "/./" segment.
	altPath := dir + string(filepath.Separator) + "." + string(filepath.Separator) + "doc.md"

	if _, _, err := d.HandleCompile(ctx, &gomcp.CallToolRequest{}, CompileInput{
		Path: altPath,
	}); err != nil {
		t.Fatalf("HandleCompile: %v", err)
	}

	after, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats after: %v", err)
	}
	if after.NodeCount != before.NodeCount {
		t.Errorf("NodeCount: before=%d, after=%d (duplicates created)", before.NodeCount, after.NodeCount)
	}
}

func TestCanonicalizePath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(sub, "doc.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		input     string
		sourceDir string
		want      string
	}{
		{
			name:  "empty source dir passes through",
			input: file, sourceDir: "", want: file,
		},
		{
			name:  "empty input passes through",
			input: "", sourceDir: dir, want: "",
		},
		{
			name:  "canonical match unchanged",
			input: file, sourceDir: dir, want: file,
		},
		{
			name:  "extra ./ collapsed to canonical",
			input: dir + "/./sub/doc.md", sourceDir: dir, want: file,
		},
		{
			name:  "outside source tree passes through",
			input: "/etc/hosts", sourceDir: dir, want: "/etc/hosts",
		},
		{
			name:  "compile root itself stays as the source dir form",
			input: dir, sourceDir: dir, want: dir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalizePath(tt.input, tt.sourceDir)

			if err != nil {
				t.Fatalf("canonicalizePath: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleDelta(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	result, _, err := d.HandleDelta(ctx, &gomcp.CallToolRequest{}, DeltaInput{})
	if err != nil {
		t.Fatalf("HandleDelta: %v", err)
	}
	if len(result.Content) == 0 {
		t.Error("empty content")
	}
}

func TestHandleSummarize(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	n := &store.Node{
		ID: "node0001", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "original", Content: "very long original content that needs summarizing",
		Format: "plain", TokenCount: 50, ContentHash: "orig_hash",
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	_, _, err := d.HandleSummarize(ctx, &gomcp.CallToolRequest{}, SummarizeInput{
		NodeID: "node0001", Summary: "short summary",
	})
	if err != nil {
		t.Fatalf("HandleSummarize: %v", err)
	}

	got, _ := st.GetNode(ctx, "node0001")
	if got.Content != "short summary" {
		t.Errorf("Content = %q, want 'short summary'", got.Content)
	}

	// summary must produce exactly one snapshot
	snaps, err := st.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if want := "summarize:node0001"; snaps[0].Message != want {
		t.Errorf("snapshot.Message = %q, want %q", snaps[0].Message, want)
	}

	diffs, err := st.GetDiffsBySnapshot(ctx, snaps[0].ID)
	if err != nil {
		t.Fatalf("GetDiffsBySnapshot: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("diffs = %d, want 1", len(diffs))
	}
	dr := diffs[0]
	if dr.Op != "mod" {
		t.Errorf("diff.Op = %q, want 'mod'", dr.Op)
	}
	if dr.NodeID != "node0001" {
		t.Errorf("diff.NodeID = %q, want 'node0001'", dr.NodeID)
	}
	if dr.OldContent != "very long original content that needs summarizing" {
		t.Errorf("diff.OldContent = %q, want pre-summarize content", dr.OldContent)
	}
	if dr.NewContent != "short summary" {
		t.Errorf("diff.NewContent = %q, want 'short summary'", dr.NewContent)
	}
}

func TestHandleHistory(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	result, _, err := d.HandleHistory(ctx, &gomcp.CallToolRequest{}, HistoryInput{
		Anchor: "nonexist",
	})
	if err != nil {
		t.Fatalf("HandleHistory: %v", err)
	}
	if len(result.Content) == 0 {
		t.Error("empty content")
	}
}

func TestHandleTree(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	root := &store.Node{
		ID: "rootroor", SourceFile: "test.md", NodeType: "heading",
		Depth: 1, Label: "Root", Content: "root",
		Format: "plain", TokenCount: 5, ContentHash: "h1",
	}
	child := &store.Node{
		ID: "child001", ParentID: "rootroor", SourceFile: "test.md", NodeType: "text",
		Depth: 2, Label: "Child", Content: "child content",
		Format: "plain", TokenCount: 10, ContentHash: "h2",
	}
	if err := st.UpsertNode(ctx, root); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if err := st.UpsertNode(ctx, child); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	result, _, err := d.HandleTree(ctx, &gomcp.CallToolRequest{}, TreeInput{})
	if err != nil {
		t.Fatalf("HandleTree: %v", err)
	}
	if len(result.Content) == 0 {
		t.Error("empty content")
	}
}

func TestHandleTree_FileFieldRelativeAndOnRootOnly(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()
	dir := t.TempDir()

	aPath := filepath.Join(dir, "a.md")
	bPath := filepath.Join(dir, "b.md")
	if err := os.WriteFile(aPath, []byte("# A heading\n\nA paragraph.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("# B heading\n\nB paragraph.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := compiler.CompileDir(ctx, st, dir, "initial"); err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	result, _, err := d.HandleTree(ctx, &gomcp.CallToolRequest{}, TreeInput{})
	if err != nil {
		t.Fatalf("HandleTree: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}

	text := result.Content[0].(*gomcp.TextContent).Text

	if !strings.Contains(text, "file=a.md") {
		t.Errorf("expected file=a.md (relative), got:\n%s", text)
	}
	if !strings.Contains(text, "file=b.md") {
		t.Errorf("expected file=b.md (relative), got:\n%s", text)
	}
	if strings.Contains(text, "file="+dir) {
		t.Errorf("absolute path leaked into output:\n%s", text)
	}

	if got := strings.Count(text, "file="); got != 2 {
		t.Errorf("expected 2 file= entries (one per file root, none on descendants), got %d. Tree:\n%s", got, text)
	}
}

func TestHandleWrite_BlocksOnOpMu(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	st.OpMu.Lock()

	done := make(chan struct{})
	go func() {
		_, _, _ = d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Payload: "hello"})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("HandleWrite completed while OpMu was held")
	case <-time.After(50 * time.Millisecond):
	}

	st.OpMu.Unlock()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HandleWrite did not complete after OpMu.Unlock")
	}
}

func TestHandleSummarize_BlocksOnOpMu(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seed := &store.Node{
		ID: "sumnode001", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "l", Content: "old",
		Format: "plain", TokenCount: 1, ContentHash: "h",
	}
	if err := st.UpsertNode(ctx, seed); err != nil {
		t.Fatal(err)
	}

	st.OpMu.Lock()

	done := make(chan struct{})
	go func() {
		_, _, _ = d.HandleSummarize(ctx, &gomcp.CallToolRequest{}, SummarizeInput{NodeID: "sumnode001", Summary: "new"})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("HandleSummarize completed while OpMu was held")
	case <-time.After(50 * time.Millisecond):
	}

	st.OpMu.Unlock()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HandleSummarize did not complete after OpMu.Unlock")
	}
}
