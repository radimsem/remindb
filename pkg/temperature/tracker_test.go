package temperature

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

func openTestDB(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func seedNode(t *testing.T, st *store.Store, id string, temp float64) {
	t.Helper()
	ctx := context.Background()
	n := &store.Node{
		ID: id, SourceFile: "test.md", NodeType: "text",
		Depth: 1, Label: "test", Content: "content",
		Format: "plain", TokenCount: 10, ContentHash: "hash" + id,
	}
	if err := st.UpsertNode(ctx, n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if err := st.UpdateTemperature(ctx, id, temp); err != nil {
		t.Fatalf("UpdateTemperature: %v", err)
	}
}

func TestRecordAccess(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "node0001", 0.5)
	seedNode(t, st, "node0002", 0.8)

	tr := NewTracker(st, DefaultConfig())

	if err := tr.RecordAccess(ctx, []string{"node0001", "node0002"}); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	n1, _ := st.GetNode(ctx, "node0001")
	if math.Abs(n1.Temperature-0.65) > 1e-10 {
		t.Errorf("node0001 temp = %g, want 0.65", n1.Temperature)
	}
	if n1.AccessCount != 1 {
		t.Errorf("node0001 access = %d, want 1", n1.AccessCount)
	}

	n2, _ := st.GetNode(ctx, "node0002")
	if math.Abs(n2.Temperature-0.95) > 1e-10 {
		t.Errorf("node0002 temp = %g, want 0.95", n2.Temperature)
	}
}

func TestRecordAccess_Cap(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "node0001", 0.95)

	tr := NewTracker(st, DefaultConfig())
	if err := tr.RecordAccess(ctx, []string{"node0001"}); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	n, _ := st.GetNode(ctx, "node0001")
	if n.Temperature != 1.0 {
		t.Errorf("temp = %f, want 1.0 (capped)", n.Temperature)
	}
}

func TestTick(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "hot00001", 0.8)
	seedNode(t, st, "cold0001", 0.05)

	cfg := DefaultConfig()
	cfg.ColdThreshold = 0.1
	tr := NewTracker(st, cfg)

	elapsed := time.Hour
	result, err := tr.Tick(ctx, elapsed)
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if result.Decayed != 2 {
		t.Errorf("Decayed = %d, want 2", result.Decayed)
	}

	hot, _ := st.GetNode(ctx, "hot00001")
	wantHot := 0.8 * math.Exp(-0.05*1.0)
	if math.Abs(hot.Temperature-wantHot) > 1e-10 {
		t.Errorf("hot temp = %f, want %f", hot.Temperature, wantHot)
	}

	cold, _ := st.GetNode(ctx, "cold0001")
	wantCold := 0.05 * math.Exp(-0.05*1.0)
	if math.Abs(cold.Temperature-wantCold) > 1e-10 {
		t.Errorf("cold temp = %f, want %f", cold.Temperature, wantCold)
	}

	if len(result.Cold) != 1 {
		t.Fatalf("Cold = %d, want 1", len(result.Cold))
	}
	if result.Cold[0].ID != "cold0001" {
		t.Errorf("Cold[0].ID = %q, want cold0001", result.Cold[0].ID)
	}
}

func TestTick_NoColdNodes(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "hot00001", 0.9)

	tr := NewTracker(st, DefaultConfig())

	result, err := tr.Tick(ctx, 10*time.Minute)
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(result.Cold) != 0 {
		t.Errorf("Cold = %d, want 0", len(result.Cold))
	}
}
