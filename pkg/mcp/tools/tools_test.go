package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
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

	d := &Deps{
		Store:   st,
		Engine:  query.NewEngine(st),
		Tracker: temperature.NewTracker(st, temperature.DefaultConfig()),
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
