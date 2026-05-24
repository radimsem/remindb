package temperature

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/store"
)

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

func mustNewTracker(t *testing.T, st NodeStore, cfg Config) *Tracker {
	t.Helper()
	tr, err := NewTracker(st, "", cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	return tr
}

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, config.Path)

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// countingStore satisfies NodeStore and tallies decay/cold-query calls so a disabled tick can be proven a no-op.
type countingStore struct {
	decays atomic.Int32
	colds  atomic.Int32
}

func (c *countingStore) BoostTemperatureBatch(context.Context, []string, float64) error {
	return nil
}

func (c *countingStore) DecayTemperatures(context.Context, float64) (int64, error) {
	c.decays.Add(1)
	return 0, nil
}

func (c *countingStore) GetColdNodes(context.Context, float64, int) ([]*store.Node, error) {
	c.colds.Add(1)
	return nil, nil
}

func TestTracker_ReloadDefaultsWhenConfigAbsent(t *testing.T) {
	dir := t.TempDir()

	bootstrap := DefaultConfig()
	bootstrap.TickInterval = 42 * time.Minute

	tr, err := NewTracker(&countingStore{}, dir, bootstrap, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	if changed := tr.reloadConfig(); changed {
		t.Error("interval should not change vs bootstrap when no config block present")
	}

	cfg, enabled := tr.snapshot()
	if !enabled {
		t.Error("enabled should default to true when block absent")
	}
	if cfg != bootstrap {
		t.Errorf("cfg = %+v, want bootstrap %+v", cfg, bootstrap)
	}
}

func TestTracker_ReloadAppliesBlock(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"temperature":{"enabled":false,"tick_interval":"5s"}}`)

	tr, err := NewTracker(&countingStore{}, dir, DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	if !tr.reloadConfig() {
		t.Error("interval changed 5m→5s, want intervalChanged=true")
	}

	cfg, enabled := tr.snapshot()
	if enabled {
		t.Error("enabled should be false from config")
	}
	if cfg.TickInterval != 5*time.Second {
		t.Errorf("TickInterval = %v, want 5s", cfg.TickInterval)
	}

	if tr.reloadConfig() {
		t.Error("re-reading identical config must report no interval change")
	}
}

func TestTracker_InvalidReloadKeepsLastGood(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"temperature":{"enabled":false,"tick_interval":"5s"}}`)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	tr, err := NewTracker(&countingStore{}, dir, DefaultConfig(), logger)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	tr.reloadConfig()
	if cfg, enabled := tr.snapshot(); enabled || cfg.TickInterval != 5*time.Second {
		t.Fatalf("precondition: good config not applied (enabled=%v interval=%v)", enabled, cfg.TickInterval)
	}

	// AccessBoost > 1 fails temperature.Config.Validate after the override.
	writeConfig(t, dir, `{"temperature":{"access_boost":2.0}}`)
	if tr.reloadConfig() {
		t.Error("invalid reload must not report an interval change")
	}

	if cfg, enabled := tr.snapshot(); enabled || cfg.TickInterval != 5*time.Second {
		t.Errorf("last-good not retained: enabled=%v interval=%v", enabled, cfg.TickInterval)
	}
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("expected WARN on invalid reload, got %q", buf.String())
	}

	// Hash was not advanced on failure, so the next valid write is picked up.
	writeConfig(t, dir, `{"temperature":{"tick_interval":"7s"}}`)
	tr.reloadConfig()
	if cfg, _ := tr.snapshot(); cfg.TickInterval != 7*time.Second {
		t.Errorf("recovery after invalid reload: TickInterval = %v, want 7s", cfg.TickInterval)
	}
}

func TestTracker_DisabledTickIsNoopThenResumes(t *testing.T) {
	dir := t.TempDir()
	cs := &countingStore{}

	tr, err := NewTracker(cs, dir, DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	writeConfig(t, dir, `{"temperature":{"enabled":false,"tick_interval":"20ms"}}`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		tr.Run(ctx, nil)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond) // many 20ms ticks while disabled
	if d, c := cs.decays.Load(), cs.colds.Load(); d != 0 || c != 0 {
		t.Fatalf("disabled loop decayed %d / queried cold %d times, want 0/0", d, c)
	}

	writeConfig(t, dir, `{"temperature":{"enabled":true,"tick_interval":"20ms"}}`)
	time.Sleep(150 * time.Millisecond) // a later tick reloads → enabled → decays

	if cs.decays.Load() == 0 {
		t.Error("re-enabling did not resume decay on a subsequent tick (no restart expected)")
	}

	cancel()
	<-done
}

func TestRecordAccess(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "node0001", 0.5)
	seedNode(t, st, "node0002", 0.8)

	tr := mustNewTracker(t, st, DefaultConfig())

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
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "node0001", 0.95)

	tr := mustNewTracker(t, st, DefaultConfig())
	if err := tr.RecordAccess(ctx, []string{"node0001"}); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	n, _ := st.GetNode(ctx, "node0001")
	if n.Temperature != 1.0 {
		t.Errorf("temp = %f, want 1.0 (capped)", n.Temperature)
	}
}

func TestTick(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "hot00001", 0.8)
	seedNode(t, st, "cold0001", 0.05)

	cfg := DefaultConfig()
	cfg.ColdThreshold = 0.1
	tr := mustNewTracker(t, st, cfg)

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
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "hot00001", 0.9)

	tr := mustNewTracker(t, st, DefaultConfig())

	result, err := tr.Tick(ctx, 10*time.Minute)
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(result.Cold) != 0 {
		t.Errorf("Cold = %d, want 0", len(result.Cold))
	}
}

func TestDedupCold_DropsWithinTTL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ColdNotifyTTL = time.Hour
	tr := mustNewTracker(t, nil, cfg)

	n := &store.Node{ID: "cold0001"}
	now := time.Now()

	if got := tr.dedupCold([]*store.Node{n}, now); len(got) != 1 {
		t.Fatalf("first tick: got %d, want 1", len(got))
	}
	tr.MarkNotified([]string{n.ID})

	if got := tr.dedupCold([]*store.Node{n}, now.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("within TTL: got %d, want 0", len(got))
	}
}

func TestDedupCold_KeepsWhenNeverMarked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ColdNotifyTTL = time.Hour
	tr := mustNewTracker(t, nil, cfg)

	n := &store.Node{ID: "cold0001"}
	now := time.Now()

	tr.dedupCold([]*store.Node{n}, now)

	if got := tr.dedupCold([]*store.Node{n}, now.Add(time.Minute)); len(got) != 1 {
		t.Fatalf("unmarked retry: got %d, want 1", len(got))
	}
}

func TestDedupCold_KeepsAfterTTL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ColdNotifyTTL = time.Hour
	tr := mustNewTracker(t, nil, cfg)

	n := &store.Node{ID: "cold0001"}
	now := time.Now()

	tr.dedupCold([]*store.Node{n}, now)
	tr.MarkNotified([]string{n.ID})

	if got := tr.dedupCold([]*store.Node{n}, now.Add(time.Hour+time.Minute)); len(got) != 1 {
		t.Fatalf("after TTL: got %d, want 1", len(got))
	}
}

func TestDedupCold_EvictsBeyondTTL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ColdNotifyTTL = time.Hour
	tr := mustNewTracker(t, nil, cfg)

	tr.MarkNotified([]string{"cold0001"})
	if len(tr.recentlyNotified) != 1 {
		t.Fatalf("after MarkNotified: map size = %d, want 1", len(tr.recentlyNotified))
	}

	tr.dedupCold(nil, time.Now().Add(time.Hour+time.Minute))
	if len(tr.recentlyNotified) != 0 {
		t.Fatalf("after eviction: map size = %d, want 0", len(tr.recentlyNotified))
	}
}
