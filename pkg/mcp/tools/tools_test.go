package tools

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/redaction"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/relations"
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

	tracker, err := temperature.NewTracker(st, "", temperature.DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	red, err := redaction.New(redaction.DefaultConfig())
	if err != nil {
		t.Fatalf("redaction.New: %v", err)
	}

	d := &Deps{
		Store:            st,
		Engine:           query.NewEngine(st),
		Resolver:         relations.New(st),
		Tracker:          tracker,
		Redactor:         red,
		SummarizeRebound: temperature.DefaultConfig().SummarizeRebound,
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

func TestHandleFetchBatch_AllFound(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustUpsertBatchNode(t, st, ctx, "aaaaaaaa", "alpha", 10)
	mustUpsertBatchNode(t, st, ctx, "bbbbbbbb", "beta", 10)
	mustUpsertBatchNode(t, st, ctx, "cccccccc", "gamma", 10)

	result, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{
		NodeIDs: []string{"bbbbbbbb", "aaaaaaaa", "cccccccc"},
	})
	if err != nil {
		t.Fatalf("HandleFetchBatch: %v", err)
	}

	text := textContent(t, result)
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "not found") || strings.Contains(text, "over budget") {
		t.Errorf("unexpected marker in all-found output:\n%s", text)
	}
}

func TestHandleFetchBatch_SomeMissing(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	n := &store.Node{
		ID: "exists01", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "exists", Content: "hello",
		Format: "plain", TokenCount: 10, ContentHash: "h_exists",
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	result, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{
		NodeIDs: []string{"exists01", "ghostid1", "ghostid2"},
	})
	if err != nil {
		t.Fatalf("HandleFetchBatch: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "hello") {
		t.Errorf("output missing returned node content:\n%s", text)
	}
	if !strings.Contains(text, "not found: ghostid1, ghostid2") {
		t.Errorf("output missing not-found marker:\n%s", text)
	}
}

func TestHandleFetchBatch_BudgetCutoff(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustUpsertBatchNode(t, st, ctx, "smallnod", "small", 10)
	mustUpsertBatchNode(t, st, ctx, "bignodee", "big", 100)

	result, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{
		NodeIDs: []string{"smallnod", "bignodee"},
		Budget:  20,
	})
	if err != nil {
		t.Fatalf("HandleFetchBatch: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "small") {
		t.Errorf("output missing small node:\n%s", text)
	}
	if !strings.Contains(text, "over budget: bignodee") {
		t.Errorf("output missing over-budget marker:\n%s", text)
	}
}

func TestHandleFetchBatch_EmptyInput(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	_, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{})
	if err == nil {
		t.Fatal("expected error for empty node_ids")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("unexpected error %v", err)
	}
}

func TestHandleFetchBatch_OverCap(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	ids := make([]string, maxFetchBatchIDs+1)
	for i := range ids {
		ids[i] = "id"
	}
	_, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{NodeIDs: ids})
	if err == nil {
		t.Fatal("expected error when exceeding cap")
	}
	if !strings.Contains(err.Error(), "exceeds cap") {
		t.Errorf("unexpected error %v", err)
	}
}

func TestHandleFetchBatch_AllOverBudget(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustUpsertBatchNode(t, st, ctx, "bignode01", "first big", 100)
	mustUpsertBatchNode(t, st, ctx, "bignode02", "second big", 100)

	result, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{
		NodeIDs: []string{"bignode01", "bignode02"},
		Budget:  10,
	})
	if err != nil {
		t.Fatalf("HandleFetchBatch: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "over budget: bignode01, bignode02") {
		t.Errorf("output missing both IDs in over-budget marker:\n%s", text)
	}
	if strings.Contains(text, "first big") || strings.Contains(text, "second big") {
		t.Errorf("output unexpectedly rendered a node body under tight budget:\n%s", text)
	}
}

func TestHandleFetchBatch_DuplicatesDoNotEcho(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustUpsertBatchNode(t, st, ctx, "bignodee", "big", 100)

	result, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{
		NodeIDs: []string{"bignodee", "bignodee"},
		Budget:  10,
	})
	if err != nil {
		t.Fatalf("HandleFetchBatch: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "over budget: bignodee") {
		t.Errorf("output missing over-budget marker:\n%s", text)
	}
	if strings.Contains(text, "bignodee, bignodee") {
		t.Errorf("duplicate ID echoed in marker:\n%s", text)
	}
}

func TestHandleStats(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustUpsertBatchNode(t, st, ctx, "aaaaaaaa", "alpha", 10)
	mustUpsertBatchNode(t, st, ctx, "bbbbbbbb", "beta", 10)

	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: "aaaaaaaa", TargetNodeID: "bbbbbbbb",
		Weight: 1.0, Origin: store.OriginManual,
	}); err != nil {
		t.Fatalf("UpsertRelation: %v", err)
	}
	if err := st.SetPinned(ctx, "aaaaaaaa", true, nil); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}

	result, _, err := d.HandleStats(ctx, &gomcp.CallToolRequest{}, StatsInput{})
	if err != nil {
		t.Fatalf("HandleStats: %v", err)
	}

	text := textContent(t, result)
	for _, want := range []string{
		"Nodes:",
		"Relations:",
		"Temperature:",
		"FTS rows:",
		"pinned:",
		store.OriginManual + ":",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("HandleStats missing %q in:\n%s", want, text)
		}
	}
}

func mustUpsertBatchNode(t *testing.T, st *store.Store, ctx context.Context, id, content string, tok int) {
	t.Helper()
	n := &store.Node{
		ID: id, SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "label " + id, Content: content,
		Format: "plain", TokenCount: tok, ContentHash: "h_" + id,
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode %s: %v", id, err)
	}
}

func textContent(t *testing.T, result *gomcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}
	tc, ok := result.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
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

func TestHandleWrite_ScrubsSecret(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	payload := "leaked key AKIAIOSFODNN7EXAMPLE in notes"
	result, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Payload: payload})
	if err != nil {
		t.Fatalf("HandleWrite: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("empty content — tool should still succeed on redaction")
	}

	stats, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.NodeCount != 1 {
		t.Fatalf("NodeCount = %d, want 1", stats.NodeCount)
	}

	res, err := d.Engine.Search(ctx, "leaked", 4000)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(res.Nodes) == 0 {
		t.Fatal("redacted node not searchable")
	}

	stored := res.Nodes[0].Node.Content
	if strings.Contains(stored, "AKIA") {
		t.Errorf("AKIA leaked into store: %q", stored)
	}
	if !strings.Contains(stored, "«redacted:aws_access_key»") {
		t.Errorf("marker missing from stored content: %q", stored)
	}
}

func TestHandleSummarize_ScrubsSecret(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	n := &store.Node{
		ID: "anchorXX", SourceFile: "x.md", NodeType: "text",
		Depth: 1, Label: "orig", Content: "original content",
		Format: "plain", TokenCount: 5, ContentHash: "h0",
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	summary := "TLDR: AKIAIOSFODNN7EXAMPLE got rotated"
	_, _, err := d.HandleSummarize(ctx, &gomcp.CallToolRequest{}, SummarizeInput{
		NodeID: "anchorXX", Summary: summary,
	})
	if err != nil {
		t.Fatalf("HandleSummarize: %v", err)
	}

	got, _ := st.GetNode(ctx, "anchorXX")
	if strings.Contains(got.Content, "AKIA") {
		t.Errorf("AKIA leaked into store: %q", got.Content)
	}
	if !strings.Contains(got.Content, "«redacted:aws_access_key»") {
		t.Errorf("marker missing from stored summary: %q", got.Content)
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

func TestHandleDiff_FullRangeAndSubset(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustWriteSnapshot(t, d, ctx, "", "first line\nsecond line")
	mustWriteSnapshot(t, d, ctx, "", "alpha\nbeta\ngamma")
	mustWriteSnapshot(t, d, ctx, "", "third one")

	snaps, err := st.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) < 3 {
		t.Fatalf("snapshots = %d, want >= 3", len(snaps))
	}

	// Snapshots come back DESC; the oldest is the last element.
	oldest := snaps[len(snaps)-1].ID
	newest := snaps[0].ID

	// Full range: from < oldest captures every snapshot up to newest.
	full, _, err := d.HandleDiff(ctx, &gomcp.CallToolRequest{}, DiffInput{
		FromSnapshotID: oldest - 1,
		ToSnapshotID:   newest,
	})
	if err != nil {
		t.Fatalf("HandleDiff full: %v", err)
	}

	fullText := textContent(t, full)
	for _, want := range []string{"[add]", "+first line", "+alpha", "+third one"} {
		if !strings.Contains(fullText, want) {
			t.Errorf("full range output missing %q\n%s", want, fullText)
		}
	}
	if strings.Contains(fullText, "(snapshot ") {
		t.Errorf("consolidated output should not carry per-record snapshot suffix\n%s", fullText)
	}

	// Tight range: only the newest snapshot.
	tight, _, err := d.HandleDiff(ctx, &gomcp.CallToolRequest{}, DiffInput{
		FromSnapshotID: newest - 1,
		ToSnapshotID:   newest,
	})
	if err != nil {
		t.Fatalf("HandleDiff tight: %v", err)
	}

	tightText := textContent(t, tight)
	if !strings.Contains(tightText, "+third one") {
		t.Errorf("tight range output missing newest diff\n%s", tightText)
	}
	if strings.Contains(tightText, "first line") || strings.Contains(tightText, "alpha") {
		t.Errorf("tight range output leaked older snapshots\n%s", tightText)
	}
}

func TestHandleDiff_ModOpEmitsGitStyleHunk(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	// First write creates the node; second write under the same anchor produces a `mod` diff.
	createMsg := mustWriteSnapshot(t, d, ctx, "", "alpha\nbeta\ngamma\n")
	nodeID := nodeIDFromWrite(t, createMsg)
	mustWriteSnapshot(t, d, ctx, nodeID, "alpha\nbeta CHANGED\ngamma\n")

	snaps, err := st.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	newest, prev := snaps[0].ID, snaps[1].ID

	result, _, err := d.HandleDiff(ctx, &gomcp.CallToolRequest{}, DiffInput{
		FromSnapshotID: prev,
		ToSnapshotID:   newest,
	})
	if err != nil {
		t.Fatalf("HandleDiff: %v", err)
	}

	text := textContent(t, result)
	for _, want := range []string{
		"[mod] " + nodeID,
		"@@",
		"-beta",
		"+beta CHANGED",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("mod output missing %q\n%s", want, text)
		}
	}
}

func TestHandleDiff_ConsolidatesMultipleMods(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	// snap1 = add v1; snap2 = mod to v2; snap3 = mod to v3.
	createMsg := mustWriteSnapshot(t, d, ctx, "", "v1 line a\nv1 line b\n")
	nodeID := nodeIDFromWrite(t, createMsg)
	mustWriteSnapshot(t, d, ctx, nodeID, "v2 line a\nv2 line b\n")
	mustWriteSnapshot(t, d, ctx, nodeID, "v3 line a\nv3 line b\n")

	snaps, err := st.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}

	// snaps is DESC; snap1 = oldest, snap3 = newest.
	snap1 := snaps[len(snaps)-1].ID
	snap3 := snaps[0].ID

	result, _, err := d.HandleDiff(ctx, &gomcp.CallToolRequest{}, DiffInput{
		FromSnapshotID: snap1,
		ToSnapshotID:   snap3,
	})
	if err != nil {
		t.Fatalf("HandleDiff: %v", err)
	}

	text := textContent(t, result)

	// One record only: the [mod] header for this node appears exactly once.
	if got := strings.Count(text, "[mod] "+nodeID); got != 1 {
		t.Errorf("expected 1 consolidated [mod] record, got %d:\n%s", got, text)
	}

	// Endpoints visible.
	for _, want := range []string{"-v1 line a", "-v1 line b", "+v3 line a", "+v3 line b"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing endpoint line %q\n%s", want, text)
		}
	}
	// Intermediate state (snap2) collapsed away.
	if strings.Contains(text, "v2") {
		t.Errorf("intermediate state should not appear in consolidated diff\n%s", text)
	}
}

func TestHandleDiff_FromGreaterThanToRejects(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	_, _, err := d.HandleDiff(ctx, &gomcp.CallToolRequest{}, DiffInput{
		FromSnapshotID: 5,
		ToSnapshotID:   3,
	})
	if err == nil {
		t.Fatal("expected error for from > to")
	}
	if !strings.Contains(err.Error(), "must be <=") {
		t.Errorf("unexpected error %v", err)
	}
}

func TestHandleDiff_EqualBoundsReturnsNoChanges(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	mustWriteSnapshot(t, d, ctx, "", "anything")

	result, _, err := d.HandleDiff(ctx, &gomcp.CallToolRequest{}, DiffInput{
		FromSnapshotID: 1,
		ToSnapshotID:   1,
	})
	if err != nil {
		t.Fatalf("HandleDiff: %v", err)
	}
	if got := textContent(t, result); got != "no changes" {
		t.Errorf("got %q, want %q", got, "no changes")
	}
}

func mustWriteSnapshot(t *testing.T, d *Deps, ctx context.Context, anchor, payload string) string {
	t.Helper()

	result, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{
		Anchor:  anchor,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("HandleWrite: %v", err)
	}

	return textContent(t, result)
}

func nodeIDFromWrite(t *testing.T, msg string) string {
	t.Helper()

	// "wrote node <id> (<n> tokens)"
	rest := strings.TrimPrefix(msg, "wrote node ")
	if rest == msg {
		t.Fatalf("unexpected write result %q", msg)
	}

	id, _, ok := strings.Cut(rest, " ")
	if !ok {
		t.Fatalf("unexpected write result %q", msg)
	}

	return id
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

func TestHandleSummarize_BumpsTemperatureToRebound(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seed := &store.Node{
		ID: "node0001", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "old", Content: "long original",
		Format: "plain", TokenCount: 50, ContentHash: "h",
	}
	if err := st.UpsertNode(ctx, seed); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if err := st.UpdateTemperature(ctx, "node0001", 0.05); err != nil {
		t.Fatalf("UpdateTemperature: %v", err)
	}

	if _, _, err := d.HandleSummarize(ctx, &gomcp.CallToolRequest{}, SummarizeInput{
		NodeID: "node0001", Summary: "short",
	}); err != nil {
		t.Fatalf("HandleSummarize: %v", err)
	}

	got, _ := st.GetNode(ctx, "node0001")
	want := temperature.DefaultConfig().SummarizeRebound
	if got.Temperature != want {
		t.Errorf("Temperature = %g, want %g", got.Temperature, want)
	}
}

func TestHandleSummarize_AcceptsTemperatureOverride(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seed := &store.Node{
		ID: "node0001", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "l", Content: "c",
		Format: "plain", TokenCount: 1, ContentHash: "h",
	}
	if err := st.UpsertNode(ctx, seed); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	override := 0.8
	if _, _, err := d.HandleSummarize(ctx, &gomcp.CallToolRequest{}, SummarizeInput{
		NodeID: "node0001", Summary: "s", Temperature: &override,
	}); err != nil {
		t.Fatalf("HandleSummarize: %v", err)
	}

	got, _ := st.GetNode(ctx, "node0001")
	if got.Temperature != 0.8 {
		t.Errorf("Temperature = %g, want 0.8", got.Temperature)
	}
}

func TestHandleSummarize_RejectsOutOfRangeTemperature(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seed := &store.Node{
		ID: "node0001", SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "l", Content: "c",
		Format: "plain", TokenCount: 1, ContentHash: "h",
	}
	if err := st.UpsertNode(ctx, seed); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	tests := []float64{-0.01, 1.01}
	for _, bad := range tests {
		if _, _, err := d.HandleSummarize(ctx, &gomcp.CallToolRequest{}, SummarizeInput{
			NodeID: "node0001", Summary: "s", Temperature: &bad,
		}); err == nil {
			t.Errorf("temperature=%g: expected error, got nil", bad)
		}
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

func mustHeading(t *testing.T, st *store.Store, id, sourceFile, label string) *store.Node {
	t.Helper()
	n := &store.Node{
		ID:          id,
		SourceFile:  sourceFile,
		NodeType:    "heading",
		Depth:       1,
		Label:       label,
		Content:     label,
		Format:      "plain",
		TokenCount:  10,
		ContentHash: "h-" + id,
	}
	if err := st.UpsertNode(context.Background(), n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	return n
}

func TestHandleRelate_ResolvedHit(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")
	tgt := mustHeading(t, st, "tgt11111111", "x.md", "Architecture")

	result, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{
		SourceID:    src.ID,
		TargetLabel: "Architecture",
		Weight:      2.5,
	})
	if err != nil {
		t.Fatalf("HandleRelate: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	if !strings.Contains(text, "resolved") {
		t.Errorf("text = %q, want contains 'resolved'", text)
	}

	related, _ := st.GetRelatedNodes(ctx, src.ID, store.DirectionOut, 1, 0, 10)
	if len(related) != 1 || related[0].Node.ID != tgt.ID {
		t.Fatalf("related = %+v, want [%s]", related, tgt.ID)
	}
	if related[0].Weight != 2.5 {
		t.Errorf("weight = %f, want 2.5", related[0].Weight)
	}
}

func TestHandleRelate_PendingMiss(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")

	result, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{
		SourceID:    src.ID,
		TargetLabel: "DoesNotExist",
	})
	if err != nil {
		t.Fatalf("HandleRelate: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	if !strings.Contains(text, "pending") {
		t.Errorf("text = %q, want contains 'pending'", text)
	}

	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	if pending[0].Origin != store.OriginManual {
		t.Errorf("origin = %s, want %s", pending[0].Origin, store.OriginManual)
	}
}

// MemoryRelate must not emit a snapshot — relations are not part of the snapshot chain.
func TestHandleRelate_DoesNotEmitSnapshot(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")
	mustHeading(t, st, "tgt11111111", "x.md", "Target")

	before, err := st.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}

	if _, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{
		SourceID: src.ID, TargetLabel: "Target",
	}); err != nil {
		t.Fatalf("HandleRelate: %v", err)
	}

	after, _ := st.ListSnapshots(ctx, 10)
	if len(after) != len(before) {
		t.Errorf("snapshot count changed: before=%d after=%d (MemoryRelate must not snapshot)", len(before), len(after))
	}
}

func TestHandleRelate_MissingSourceFails(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	_, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{
		SourceID:    "ghost111111",
		TargetLabel: "anything",
	})
	if err == nil {
		t.Fatal("expected error for missing source_id, got nil")
	}
	if !strings.Contains(err.Error(), "source_id not found") {
		t.Errorf("err = %v, want contains 'source_id not found'", err)
	}
}

func TestHandleRelate_NoTargetFails(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")

	_, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{SourceID: src.ID})
	if err == nil {
		t.Fatal("expected error when neither target_id nor target_label given")
	}
}

func TestHandleRelated_ReturnsTargets(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")
	tgt := mustHeading(t, st, "tgt11111111", "y.md", "Target")

	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: src.ID, TargetNodeID: tgt.ID, Weight: 1.5, Origin: store.OriginParsed,
	}); err != nil {
		t.Fatalf("UpsertRelation: %v", err)
	}

	result, _, err := d.HandleRelated(ctx, &gomcp.CallToolRequest{}, RelatedInput{
		Anchor: src.ID, Direction: store.DirectionOut,
	})
	if err != nil {
		t.Fatalf("HandleRelated: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	if !strings.Contains(text, tgt.ID) {
		t.Errorf("text = %q, want contains %s", text, tgt.ID)
	}
	if !strings.Contains(text, "Target") {
		t.Errorf("text missing target label: %q", text)
	}
}

func TestHandleRelated_Empty(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")

	result, _, err := d.HandleRelated(ctx, &gomcp.CallToolRequest{}, RelatedInput{Anchor: src.ID})
	if err != nil {
		t.Fatalf("HandleRelated: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	if text != "no related nodes" {
		t.Errorf("text = %q, want 'no related nodes'", text)
	}
}

func TestHandleRelated_AnchorRequired(t *testing.T) {
	d, _ := setup(t)
	_, _, err := d.HandleRelated(context.Background(), &gomcp.CallToolRequest{}, RelatedInput{})
	if err == nil {
		t.Fatal("expected error for missing anchor")
	}
}

// MemoryRelated must not take Store.OpMu (read tools don't lock).
func TestHandleRelated_DoesNotLockOpMu(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")

	// Hold OpMu and confirm HandleRelated still returns promptly.
	st.OpMu.Lock()
	defer st.OpMu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = d.HandleRelated(ctx, &gomcp.CallToolRequest{}, RelatedInput{Anchor: src.ID})
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HandleRelated blocked on OpMu (read tools must not lock)")
	}
}

func TestHandleRelate_TargetIDMissGoesPending(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")
	mustHeading(t, st, "real1111111", "x.md", "RealHeading")

	result, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{
		SourceID:    src.ID,
		TargetID:    "ghost111111",
		TargetLabel: "RealHeading", // would match if label-fallback existed
	})
	if err != nil {
		t.Fatalf("HandleRelate: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	if !strings.Contains(text, "pending") {
		t.Errorf("text = %q, want contains 'pending' (no fallback when target_id misses)", text)
	}

	// No relations row should have been created.
	related, _ := st.GetRelatedNodes(ctx, src.ID, store.DirectionOut, 1, 0, 10)
	if len(related) != 0 {
		t.Errorf("got %d related, want 0 (target_id miss must not silently pick a label match)", len(related))
	}
}

func TestHandleRelate_SelfLoopExcludedFromResults(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	a := mustHeading(t, st, "selflp11111", "x.md", "Self")

	if _, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{
		SourceID: a.ID, TargetLabel: "Self",
	}); err != nil {
		t.Fatalf("HandleRelate: %v", err)
	}

	result, _, _ := d.HandleRelated(ctx, &gomcp.CallToolRequest{}, RelatedInput{Anchor: a.ID})
	text := result.Content[0].(*gomcp.TextContent).Text
	if text != "no related nodes" {
		t.Errorf("text = %q, want 'no related nodes' (self-loop must not surface as related)", text)
	}
}

func TestHandleRelated_CycleTerminates(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	a := mustHeading(t, st, "aaa11111111", "x.md", "A")
	b := mustHeading(t, st, "bbb11111111", "x.md", "B")

	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: a.ID, TargetNodeID: b.ID, Weight: 1.0, Origin: store.OriginParsed,
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: b.ID, TargetNodeID: a.ID, Weight: 1.0, Origin: store.OriginParsed,
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = d.HandleRelated(ctx, &gomcp.CallToolRequest{}, RelatedInput{
			Anchor: a.ID, Direction: store.DirectionBoth, Depth: 5,
		})
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("HandleRelated did not terminate on a cycle (depth cap should bound the CTE)")
	}
}

// An unknown direction string falls through to the default behavior (DirectionBoth).
func TestHandleRelated_InvalidDirectionDefaults(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	a := mustHeading(t, st, "aaa11111111", "x.md", "A")
	b := mustHeading(t, st, "bbb11111111", "x.md", "B")

	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: a.ID, TargetNodeID: b.ID, Weight: 1.0, Origin: store.OriginParsed,
	}); err != nil {
		t.Fatal(err)
	}

	result, _, err := d.HandleRelated(ctx, &gomcp.CallToolRequest{}, RelatedInput{
		Anchor: a.ID, Direction: "upward",
	})
	if err != nil {
		t.Fatalf("HandleRelated: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	if !strings.Contains(text, b.ID) {
		t.Errorf("unknown direction should default to 'both' and surface b; text = %q", text)
	}
}

func TestHandleRelate_ManualEdgeSurvivesResolverRun(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source")
	mustHeading(t, st, "tgt11111111", "x.md", "Target")

	if _, _, err := d.HandleRelate(ctx, &gomcp.CallToolRequest{}, RelateInput{
		SourceID: src.ID, TargetLabel: "Target", Weight: 2.0,
	}); err != nil {
		t.Fatalf("HandleRelate: %v", err)
	}

	if err := relations.Run(ctx, st, nil); err != nil {
		t.Fatalf("relations.Run: %v", err)
	}

	related, _ := st.GetRelatedNodes(ctx, src.ID, store.DirectionOut, 1, 0, 10)
	if len(related) != 1 {
		t.Fatalf("manual edge lost after resolver run: %+v", related)
	}
	if related[0].Weight != 2.0 {
		t.Errorf("manual edge weight changed: got %f, want 2.0", related[0].Weight)
	}
}

func TestHandlePin(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustHeading(t, st, "pinnode01111", "x.md", "Important")

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}

	result, _, err := d.HandlePin(ctx, &gomcp.CallToolRequest{}, PinInput{NodeID: "pinnode01111"})
	if err != nil {
		t.Fatalf("HandlePin: %v", err)
	}
	if !strings.Contains(textContent(t, result), "pinned node pinnode01111") {
		t.Errorf("unexpected message: %q", textContent(t, result))
	}

	got, err := st.GetNode(ctx, "pinnode01111")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if !got.Pinned {
		t.Error("Pinned = false after HandlePin")
	}

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapsAfter) != len(snapsBefore) {
		t.Errorf("snapshot count changed: before=%d after=%d (pin must not emit)", len(snapsBefore), len(snapsAfter))
	}
}

func TestHandlePin_EmptyNodeID(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	_, _, err := d.HandlePin(ctx, &gomcp.CallToolRequest{}, PinInput{NodeID: ""})
	if err == nil {
		t.Fatal("HandlePin with empty node_id should error")
	}
	if !strings.Contains(err.Error(), "node_id is required") {
		t.Errorf("err = %q, want node_id-required message", err)
	}
}

func TestHandlePin_MissingNode(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	_, _, err := d.HandlePin(ctx, &gomcp.CallToolRequest{}, PinInput{NodeID: "ghost1234567"})
	if err == nil {
		t.Fatal("HandlePin on missing node should error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %q, want not-found message", err)
	}
}

func TestHandlePin_WithTemperature(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustHeading(t, st, "pinnode03333", "x.md", "Important")
	must(t, st.UpdateTemperature(ctx, "pinnode03333", 0.2))

	target := 0.85
	result, _, err := d.HandlePin(ctx, &gomcp.CallToolRequest{}, PinInput{
		NodeID:      "pinnode03333",
		Temperature: &target,
	})
	if err != nil {
		t.Fatalf("HandlePin: %v", err)
	}
	if !strings.Contains(textContent(t, result), "at temperature 0.85") {
		t.Errorf("unexpected message: %q", textContent(t, result))
	}

	got, err := st.GetNode(ctx, "pinnode03333")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if !got.Pinned {
		t.Error("Pinned = false after HandlePin with temperature")
	}
	if got.Temperature != 0.85 {
		t.Errorf("Temperature = %f, want 0.85 (override applied)", got.Temperature)
	}
}

func TestHandlePin_TemperatureOutOfRange(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustHeading(t, st, "pinnode04444", "x.md", "Important")

	bad := 1.5
	_, _, err := d.HandlePin(ctx, &gomcp.CallToolRequest{}, PinInput{
		NodeID:      "pinnode04444",
		Temperature: &bad,
	})
	if err == nil {
		t.Fatal("HandlePin with temperature 1.5 should error")
	}
	if !strings.Contains(err.Error(), "temperature must be in [0, 1]") {
		t.Errorf("err = %q, want range-violation message", err)
	}
}

func TestHandleUnpin(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	mustHeading(t, st, "pinnode02222", "x.md", "Important")
	must(t, st.SetPinned(ctx, "pinnode02222", true, nil))

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}

	result, _, err := d.HandleUnpin(ctx, &gomcp.CallToolRequest{}, UnpinInput{NodeID: "pinnode02222"})
	if err != nil {
		t.Fatalf("HandleUnpin: %v", err)
	}
	if !strings.Contains(textContent(t, result), "unpinned node pinnode02222") {
		t.Errorf("unexpected message: %q", textContent(t, result))
	}

	got, err := st.GetNode(ctx, "pinnode02222")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Pinned {
		t.Error("Pinned = true after HandleUnpin")
	}

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapsAfter) != len(snapsBefore) {
		t.Errorf("snapshot count changed: before=%d after=%d (unpin must not emit)", len(snapsBefore), len(snapsAfter))
	}
}

func TestHandleForget_Strict_Leaf(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seedForget(t, st, "fgleaf0001", "")

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	must(t, err)

	result, _, err := d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "fgleaf0001"})
	if err != nil {
		t.Fatalf("HandleForget: %v", err)
	}
	if !strings.Contains(textContent(t, result), "forgot node fgleaf0001") {
		t.Errorf("unexpected message: %q", textContent(t, result))
	}

	if _, err := st.GetNode(ctx, "fgleaf0001"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("node still present after forget; err = %v", err)
	}

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if got := len(snapsAfter) - len(snapsBefore); got != 1 {
		t.Errorf("snapshot count delta = %d, want 1 (forget must emit exactly one)", got)
	}
}

func TestHandleForget_Strict_RejectsParent(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seedForget(t, st, "fgroot00001", "")
	seedForget(t, st, "fgchild0001", "fgroot00001")

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	must(t, err)

	_, _, err = d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "fgroot00001", Mode: "strict"})
	if err == nil {
		t.Fatal("HandleForget strict on parent: want error, got nil")
	}
	if !strings.Contains(err.Error(), "has 1 children") {
		t.Errorf("err = %q, want children-count message", err)
	}

	for _, id := range []string{"fgroot00001", "fgchild0001"} {
		if _, err := st.GetNode(ctx, id); err != nil {
			t.Errorf("node %s unexpectedly removed: %v", id, err)
		}
	}

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if len(snapsAfter) != len(snapsBefore) {
		t.Errorf("snapshot count changed on rejected forget: before=%d after=%d", len(snapsBefore), len(snapsAfter))
	}
}

func TestHandleForget_Cascade_EmitsSingleSnapshot(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seedForget(t, st, "fgroot00002", "")
	seedForget(t, st, "fgmid000002", "fgroot00002")
	seedForget(t, st, "fgleaf0002a", "fgmid000002")
	seedForget(t, st, "fgleaf0002b", "fgmid000002")

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	must(t, err)

	_, _, err = d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "fgmid000002", Mode: "cascade"})
	if err != nil {
		t.Fatalf("HandleForget cascade: %v", err)
	}

	for _, id := range []string{"fgmid000002", "fgleaf0002a", "fgleaf0002b"} {
		if _, err := st.GetNode(ctx, id); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("node %s still present after cascade; err = %v", id, err)
		}
	}

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if got := len(snapsAfter) - len(snapsBefore); got != 1 {
		t.Errorf("snapshot count delta = %d, want 1 (cascade must emit exactly one)", got)
	}
}

func TestHandleForget_Reparent_EmitsSingleSnapshot(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seedForget(t, st, "fgroot00003", "")
	seedForget(t, st, "fgmid000003", "fgroot00003")
	seedForget(t, st, "fgleaf0003a", "fgmid000003")
	seedForget(t, st, "fgleaf0003b", "fgmid000003")

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	must(t, err)

	_, _, err = d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "fgmid000003", Mode: "reparent"})
	if err != nil {
		t.Fatalf("HandleForget reparent: %v", err)
	}

	for _, id := range []string{"fgleaf0003a", "fgleaf0003b"} {
		n, err := st.GetNode(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}

		if n.ParentID != "fgroot00003" {
			t.Errorf("%s parent = %q, want fgroot00003", id, n.ParentID)
		}
	}

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if got := len(snapsAfter) - len(snapsBefore); got != 1 {
		t.Errorf("snapshot count delta = %d, want 1 (reparent must emit exactly one)", got)
	}
}

func TestHandleForget_MissingNode(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	_, _, err := d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "ghost1234567"})
	if err == nil {
		t.Fatal("HandleForget on missing node: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %q, want not-found message", err)
	}
}

func TestHandleForget_InvalidMode(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seedForget(t, st, "fgleaf0004", "")

	_, _, err := d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "fgleaf0004", Mode: "bogus"})
	if err == nil {
		t.Fatal("HandleForget with invalid mode: want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown delete mode") {
		t.Errorf("err = %q, want unknown-mode message", err)
	}
}

func TestHandleForget_BlocksOnOpMu(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	seedForget(t, st, "fgleaf0005", "")

	st.OpMu.Lock()

	done := make(chan struct{})
	go func() {
		_, _, _ = d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "fgleaf0005"})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("HandleForget completed while OpMu was held")
	case <-time.After(50 * time.Millisecond):
	}

	st.OpMu.Unlock()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HandleForget did not complete after OpMu.Unlock")
	}
}

func writeAndSnap(t *testing.T, d *Deps, payload string) (nodeID string, snapID int64) {
	t.Helper()
	ctx := context.Background()

	res, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Payload: payload})
	if err != nil {
		t.Fatalf("HandleWrite: %v", err)
	}

	text := textContent(t, res)
	nodeID = extractWrittenNodeID(text)
	if nodeID == "" {
		t.Fatalf("could not parse node id from write result: %q", text)
	}

	snaps, err := d.Store.ListSnapshots(ctx, 1)
	if err != nil || len(snaps) == 0 {
		t.Fatalf("ListSnapshots: %v (len=%d)", err, len(snaps))
	}
	return nodeID, snaps[0].ID
}

// "wrote node <id> (N tokens)" → "<id>"
func extractWrittenNodeID(s string) string {
	const prefix = "wrote node "
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}

	rest := s[i+len(prefix):]
	end := strings.IndexByte(rest, ' ')
	if end < 0 {
		return rest
	}

	return rest[:end]
}

func TestHandleRollback_RestoresMutatedContent(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	nodeID, snap1 := writeAndSnap(t, d, "Title alpha\nbody v1\n")

	_, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Anchor: nodeID, Payload: "Title alpha\nbody v2\n"})
	must(t, err)

	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	got, err := st.GetNode(ctx, nodeID)
	must(t, err)

	if !strings.Contains(got.Content, "body v1") {
		t.Errorf("content after rollback = %q, want it to contain 'body v1'", got.Content)
	}
	if strings.Contains(got.Content, "body v2") {
		t.Errorf("content after rollback still has 'body v2': %q", got.Content)
	}
}

func TestHandleRollback_RemovesNodeAddedAfterTarget(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	idA, snap1 := writeAndSnap(t, d, "Title alpha\nfirst note\n")
	idB, _ := writeAndSnap(t, d, "Title beta\nsecond note\n")

	_, _, err := d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	if _, err := st.GetNode(ctx, idA); err != nil {
		t.Errorf("nodeA disappeared after rollback: %v", err)
	}
	if _, err := st.GetNode(ctx, idB); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("nodeB still present after rollback; err = %v", err)
	}
}

func TestHandleRollback_RestoresDeletedNode(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	id, snap1 := writeAndSnap(t, d, "Title alpha\nbody to delete\n")

	_, _, err := d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: id, Mode: "cascade"})
	must(t, err)

	if _, err := st.GetNode(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected node to be deleted, got err = %v", err)
	}

	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	got, err := st.GetNode(ctx, id)
	if err != nil {
		t.Fatalf("nodeA gone after rollback that should restore it: %v", err)
	}
	if !strings.Contains(got.Content, "body to delete") {
		t.Errorf("restored content = %q, want it to contain 'body to delete'", got.Content)
	}
}

func TestHandleRollback_DropAfterTrue_PrunesIntermediate(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	idAlpha, snap1 := writeAndSnap(t, d, "Title alpha\nfirst\n")
	idBeta, _ := writeAndSnap(t, d, "Title beta\nsecond\n")
	idGamma, _ := writeAndSnap(t, d, "Title gamma\nthird\n")

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if len(snapsBefore) != 3 {
		t.Fatalf("expected 3 snapshots before rollback, got %d", len(snapsBefore))
	}

	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1, DropAfter: true})
	must(t, err)

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if len(snapsAfter) != 2 {
		t.Fatalf("expected 2 snapshots after drop_after rollback (snap1 + rollback), got %d", len(snapsAfter))
	}

	// DESC order: rollback then snap1.
	rollbackSnap := snapsAfter[0]
	if !strings.HasPrefix(rollbackSnap.Message, "rollback to ") {
		t.Errorf("HEAD message = %q, want it to start with 'rollback to '", rollbackSnap.Message)
	}
	if !rollbackSnap.ParentID.Valid || rollbackSnap.ParentID.Int64 != snap1 {
		t.Errorf("rollback snap parent = %v, want %d (drop_after=true linearizes to target)", rollbackSnap.ParentID, snap1)
	}

	// Node graph at snap1 held only alpha; beta and gamma were added later and must be gone.
	alpha, err := st.GetNode(ctx, idAlpha)
	if err != nil {
		t.Fatalf("alpha missing after rollback: %v", err)
	}
	if !strings.Contains(alpha.Content, "first") {
		t.Errorf("alpha content = %q, want it to contain 'first'", alpha.Content)
	}
	for _, id := range []string{idBeta, idGamma} {
		if _, err := st.GetNode(ctx, id); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("node %s still present after rollback; err = %v", id, err)
		}
	}
}

func TestHandleRollback_DropAfterFalse_KeepsIntermediate(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	idAlpha, snap1 := writeAndSnap(t, d, "Title alpha\nfirst\n")
	idBeta, _ := writeAndSnap(t, d, "Title beta\nsecond\n")
	idGamma, snap3 := writeAndSnap(t, d, "Title gamma\nthird\n")

	_, _, err := d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if len(snapsAfter) != 4 {
		t.Fatalf("expected 4 snapshots after non-pruning rollback, got %d", len(snapsAfter))
	}

	rollbackSnap := snapsAfter[0]
	if !rollbackSnap.ParentID.Valid || rollbackSnap.ParentID.Int64 != snap3 {
		t.Errorf("rollback snap parent = %v, want %d (drop_after=false hangs off prev HEAD)", rollbackSnap.ParentID, snap3)
	}

	alpha, err := st.GetNode(ctx, idAlpha)
	if err != nil {
		t.Fatalf("alpha missing after rollback: %v", err)
	}

	if !strings.Contains(alpha.Content, "first") {
		t.Errorf("alpha content = %q, want it to contain 'first'", alpha.Content)
	}
	for _, id := range []string{idBeta, idGamma} {
		if _, err := st.GetNode(ctx, id); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("node %s still present after rollback; err = %v", id, err)
		}
	}
}

func TestHandleRollback_NextWriteChainsOnRollback(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	_, snap1 := writeAndSnap(t, d, "Title alpha\nfirst\n")
	_, _ = writeAndSnap(t, d, "Title beta\nsecond\n")

	_, _, err := d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	rollbackHead, err := st.GetHeadSnapshotID(ctx)
	must(t, err)

	_, _, err = d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Payload: "Title delta\npost-rollback\n"})
	must(t, err)

	snaps, err := st.ListSnapshots(ctx, 1)
	must(t, err)
	if len(snaps) == 0 {
		t.Fatal("no snapshot after post-rollback write")
	}

	post := snaps[0]
	if !post.ParentID.Valid || post.ParentID.Int64 != rollbackHead {
		t.Errorf("post-rollback write parent = %v, want %d (rollback snap)", post.ParentID, rollbackHead)
	}
}

func TestHandleRollback_NoOpWhenTargetIsHead(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	_, snap1 := writeAndSnap(t, d, "Title alpha\nbody\n")

	res, _, err := d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)
	if !strings.Contains(textContent(t, res), "already at snapshot") {
		t.Errorf("expected 'already at snapshot' message, got %q", textContent(t, res))
	}

	snaps, err := st.ListSnapshots(ctx, 100)
	must(t, err)
	if len(snaps) != 1 {
		t.Errorf("snapshot count = %d, want 1 (no-op must not emit)", len(snaps))
	}
}

func TestHandleRollback_RejectsInvalidInput(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		snapID int64
	}{
		{"zero", 0},
		{"negative", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: tc.snapID})

			if err == nil {
				t.Fatalf("expected error for snapshot_id=%d, got nil", tc.snapID)
			}
		})
	}
}

func TestHandleRollback_RejectsNonexistentSnapshot(t *testing.T) {
	d, _ := setup(t)
	ctx := context.Background()

	_, _ = writeAndSnap(t, d, "warm-up\n")

	_, _, err := d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: 999})
	if err == nil {
		t.Fatal("expected error for nonexistent snapshot, got nil")
	}
}

func TestHandleRollback_BlocksOnOpMu(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	_, snap1 := writeAndSnap(t, d, "Title alpha\nbody\n")
	_, _ = writeAndSnap(t, d, "Title beta\nbody\n")

	st.OpMu.Lock()
	done := make(chan error, 1)
	go func() {
		_, _, err := d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
		done <- err
	}()

	select {
	case <-done:
		t.Fatal("HandleRollback completed while OpMu was held")
	case <-time.After(50 * time.Millisecond):
	}

	st.OpMu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleRollback after unlock: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HandleRollback did not complete after OpMu.Unlock")
	}
}

func TestHandleRollback_RestoresSubtreeAfterCascade(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	// Seed a 3-deep tree directly via UpsertNode — no snapshot yet.
	seedForget(t, st, "rootid01234", "")
	seedForget(t, st, "midid012345", "rootid01234")
	seedForget(t, st, "leafid01234", "midid012345")

	_, baseline := writeAndSnap(t, d, "Title baseline\nwarmup body\n")

	_, _, err := d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "rootid01234", Mode: "cascade"})
	must(t, err)

	for _, id := range []string{"rootid01234", "midid012345", "leafid01234"} {
		if _, err := st.GetNode(ctx, id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("subtree node %s still present after cascade: %v", id, err)
		}
	}

	// Roll back to the baseline.
	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: baseline})
	must(t, err)

	// All three nodes must be back with the right parent chain.
	root, err := st.GetNode(ctx, "rootid01234")
	must(t, err)
	if root.ParentID != "" {
		t.Errorf("rootid parent = %q, want empty (root)", root.ParentID)
	}

	mid, err := st.GetNode(ctx, "midid012345")
	must(t, err)
	if mid.ParentID != "rootid01234" {
		t.Errorf("midid parent = %q, want rootid01234", mid.ParentID)
	}

	leaf, err := st.GetNode(ctx, "leafid01234")
	must(t, err)
	if leaf.ParentID != "midid012345" {
		t.Errorf("leafid parent = %q, want midid012345", leaf.ParentID)
	}
}

func TestHandleRollback_ChainOfModsReversesToOriginal(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	id, snap1 := writeAndSnap(t, d, "Title alpha\noriginal body\n")

	for _, payload := range []string{
		"Title alpha\nfirst edit\n",
		"Title alpha\nsecond edit\n",
		"Title alpha\nthird edit\n",
	} {
		_, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Anchor: id, Payload: payload})
		must(t, err)
	}

	current, err := st.GetNode(ctx, id)
	must(t, err)
	if !strings.Contains(current.Content, "third edit") {
		t.Fatalf("pre-rollback content = %q, want third edit", current.Content)
	}

	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	restored, err := st.GetNode(ctx, id)
	must(t, err)

	if !strings.Contains(restored.Content, "original body") {
		t.Errorf("after chain rollback, content = %q, want original body", restored.Content)
	}
	for _, leaked := range []string{"first edit", "second edit", "third edit"} {
		if strings.Contains(restored.Content, leaked) {
			t.Errorf("intermediate content %q leaked into rolled-back state: %q", leaked, restored.Content)
		}
	}
}

func TestHandleRollback_FTSReflectsRestoredContent(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	id, snap1 := writeAndSnap(t, d, "Title alpha\nUNIQUEMARKERORIGINAL\n")

	_, _, err := d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Anchor: id, Payload: "Title alpha\nUNIQUEMARKERPOSTTARGET\n"})
	must(t, err)

	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	hits, err := st.Search(ctx, "UNIQUEMARKERORIGINAL", 10)
	must(t, err)
	if len(hits) == 0 {
		t.Errorf("FTS5 missed the restored content after rollback — triggers didn't fire")
	}

	stale, err := st.Search(ctx, "UNIQUEMARKERPOSTTARGET", 10)
	must(t, err)
	if len(stale) != 0 {
		t.Errorf("FTS5 still indexes the rolled-away content: %d hits", len(stale))
	}
}

func TestHandleRollback_ReparentReversal(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	// Tree: root → mid → leafA, leafB
	seedForget(t, st, "rprt00root1", "")
	seedForget(t, st, "rprt000mid1", "rprt00root1")
	seedForget(t, st, "rprt0leafa1", "rprt000mid1")
	seedForget(t, st, "rprt0leafb1", "rprt000mid1")

	_, baseline := writeAndSnap(t, d, "Title baseline\nwarmup\n")

	// Reparent: leaves get promoted to root, mid is deleted.
	_, _, err := d.HandleForget(ctx, &gomcp.CallToolRequest{}, ForgetInput{NodeID: "rprt000mid1", Mode: "reparent"})
	must(t, err)

	leafA, err := st.GetNode(ctx, "rprt0leafa1")
	must(t, err)

	if leafA.ParentID != "rprt00root1" {
		t.Fatalf("after reparent, leafA.parent = %q, want rprt00root1", leafA.ParentID)
	}
	if _, err := st.GetNode(ctx, "rprt000mid1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("mid not deleted after reparent: %v", err)
	}

	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: baseline})
	must(t, err)

	mid, err := st.GetNode(ctx, "rprt000mid1")
	must(t, err)
	if mid.ParentID != "rprt00root1" {
		t.Errorf("after rollback, mid.parent = %q, want rprt00root1", mid.ParentID)
	}

	for _, id := range []string{"rprt0leafa1", "rprt0leafb1"} {
		n, err := st.GetNode(ctx, id)
		must(t, err)

		if n.ParentID != "rprt000mid1" {
			t.Errorf("after rollback, %s.parent = %q, want rprt000mid1", id, n.ParentID)
		}
	}
}

func TestHandleRollback_PreservesTemperatureOnRestoredNode(t *testing.T) {
	d, st := setup(t)
	ctx := context.Background()

	id, snap1 := writeAndSnap(t, d, "Title alpha\nbody\n")

	// Boost temperature past the default.
	must(t, d.Tracker.RecordAccess(ctx, []string{id}))
	before, err := st.GetNode(ctx, id)
	must(t, err)
	beforeTemp := before.Temperature

	_, _, err = d.HandleWrite(ctx, &gomcp.CallToolRequest{}, WriteInput{Anchor: id, Payload: "Title alpha\nedited\n"})
	must(t, err)

	_, _, err = d.HandleRollback(ctx, &gomcp.CallToolRequest{}, RollbackInput{SnapshotID: snap1})
	must(t, err)

	after, err := st.GetNode(ctx, id)
	must(t, err)
	if after.Temperature != beforeTemp {
		t.Errorf("temperature after rollback = %g, want preserved %g", after.Temperature, beforeTemp)
	}
}

func seedForget(t *testing.T, st *store.Store, id, parent string) {
	t.Helper()

	n := &store.Node{
		ID: id, ParentID: parent,
		SourceFile: "test.md", NodeType: "heading", Depth: 1,
		Label: "label " + id, Content: "content " + id,
		Format: "plain", TokenCount: 5, ContentHash: "h-" + id,
	}

	if err := st.UpsertNode(context.Background(), n); err != nil {
		t.Fatalf("seedForget %s: %v", id, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func intPtr(n int) *int { return &n }

func TestResolveBudget(t *testing.T) {
	cases := []struct {
		name    string
		arg     int
		cfg     *int
		builtin int
		want    int
	}{
		{"explicit arg wins over config", 700, intPtr(300), 1000, 700},
		{"explicit arg wins over builtin", 700, nil, 1000, 700},
		{"config used when arg absent", 0, intPtr(300), 1000, 300},
		{"builtin used when arg absent and config nil", 0, nil, 1000, 1000},
		{"builtin used when config non-positive", 0, intPtr(0), 1000, 1000},
		{"negative arg falls through to config", -5, intPtr(300), 1000, 300},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveBudget(c.arg, c.cfg, c.builtin); got != c.want {
				t.Errorf("resolveBudget(%d, %v, %d) = %d, want %d", c.arg, c.cfg, c.builtin, got, c.want)
			}
		})
	}
}

func seedFetchTree(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()

	root := &store.Node{
		ID: "rootnode", SourceFile: "t.md", NodeType: "text",
		Depth: 1, Label: "root body", Content: "root body",
		Format: "plain", TokenCount: 10, ContentHash: "h_root",
	}
	child := &store.Node{
		ID: "childnode", ParentID: "rootnode", SourceFile: "t.md", NodeType: "text",
		Depth: 2, Label: "child body", Content: "child body",
		Format: "plain", TokenCount: 100, ContentHash: "h_child",
	}

	must(t, st.UpsertNode(ctx, root))
	must(t, st.UpsertNode(ctx, child))
}

func TestHandleFetch_ConfigDefaultAppliedWhenArgAbsent(t *testing.T) {
	d, st := setup(t)
	seedFetchTree(t, st)
	d.WorkspaceConfig = config.Config{Budgets: config.BudgetsConfig{Fetch: intPtr(200)}}

	result, _, err := d.HandleFetch(context.Background(), &gomcp.CallToolRequest{}, FetchInput{
		Anchor: "rootnode", // Budget omitted -> config default 200 (200-10 >= 100)
	})
	if err != nil {
		t.Fatalf("HandleFetch: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "child body") {
		t.Errorf("config default budget should have admitted the child:\n%s", text)
	}
}

func TestHandleFetch_ExplicitArgOverridesConfigDefault(t *testing.T) {
	d, st := setup(t)
	seedFetchTree(t, st)
	d.WorkspaceConfig = config.Config{Budgets: config.BudgetsConfig{Fetch: intPtr(200)}}

	result, _, err := d.HandleFetch(context.Background(), &gomcp.CallToolRequest{}, FetchInput{
		Anchor: "rootnode", Budget: 50, // explicit 50 wins over config 200 (50-10 < 100)
	})
	if err != nil {
		t.Fatalf("HandleFetch: %v", err)
	}

	text := textContent(t, result)
	if strings.Contains(text, "child body") {
		t.Errorf("explicit budget should have excluded the child:\n%s", text)
	}
	if !strings.Contains(text, "root body") {
		t.Errorf("anchor should always be present:\n%s", text)
	}
}

func TestHandleFetchBatch_BudgetResolution(t *testing.T) {
	ctx := context.Background()
	ids := []string{"rootnode", "childnode"}

	t.Run("config default trims when arg absent", func(t *testing.T) {
		d, st := setup(t)
		seedFetchTree(t, st)
		d.WorkspaceConfig = config.Config{Budgets: config.BudgetsConfig{FetchBatch: intPtr(50)}}

		res, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{NodeIDs: ids})
		if err != nil {
			t.Fatalf("HandleFetchBatch: %v", err)
		}
		if txt := textContent(t, res); strings.Contains(txt, "child body") {
			t.Errorf("config default 50 should trim the 100-tok child:\n%s", txt)
		}
	})

	t.Run("explicit arg overrides config default", func(t *testing.T) {
		d, st := setup(t)
		seedFetchTree(t, st)
		d.WorkspaceConfig = config.Config{Budgets: config.BudgetsConfig{FetchBatch: intPtr(50)}}

		res, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{NodeIDs: ids, Budget: 5000})
		if err != nil {
			t.Fatalf("HandleFetchBatch: %v", err)
		}
		if txt := textContent(t, res); !strings.Contains(txt, "child body") {
			t.Errorf("explicit 5000 should override config 50:\n%s", txt)
		}
	})

	t.Run("no arg and no config is unlimited", func(t *testing.T) {
		d, st := setup(t)
		seedFetchTree(t, st)

		res, _, err := d.HandleFetchBatch(ctx, &gomcp.CallToolRequest{}, FetchBatchInput{NodeIDs: ids})
		if err != nil {
			t.Fatalf("HandleFetchBatch: %v", err)
		}
		if txt := textContent(t, res); !strings.Contains(txt, "child body") {
			t.Errorf("no arg + no config should be unlimited and include the child:\n%s", txt)
		}
	})
}

func TestHandleSearch_NoArgNoConfigIsUnlimited(t *testing.T) {
	d, st := setup(t)
	seedFetchTree(t, st)

	res, _, err := d.HandleSearch(context.Background(), &gomcp.CallToolRequest{}, SearchInput{Query: "body"})
	if err != nil {
		t.Fatalf("HandleSearch: %v", err)
	}

	txt := textContent(t, res)
	if strings.Contains(txt, "no results") {
		t.Errorf("Search with no budget arg and no config must not return empty (unlimited):\n%s", txt)
	}
	if !strings.Contains(txt, "root body") && !strings.Contains(txt, "child body") {
		t.Errorf("Search should have surfaced the seeded nodes:\n%s", txt)
	}
}

func TestBudgetResolution_PerToolIndependence(t *testing.T) {
	d, st := setup(t)
	seedFetchTree(t, st)

	// Search default is tiny; Fetch default is generous.
	d.WorkspaceConfig = config.Config{Budgets: config.BudgetsConfig{
		Search: intPtr(5),
		Fetch:  intPtr(200),
	}}

	result, _, err := d.HandleFetch(context.Background(), &gomcp.CallToolRequest{}, FetchInput{
		Anchor: "rootnode",
	})
	if err != nil {
		t.Fatalf("HandleFetch: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "child body") {
		t.Errorf("Fetch resolved the wrong config field (used Search=5 not Fetch=200):\n%s", text)
	}
}
