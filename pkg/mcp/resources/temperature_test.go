package resources

import (
	"math"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

// temps sorted: [0.02, 0.05, 0.30, 0.80] → lower-median at index 2 = 0.30.
func temperatureFixture() []*store.Node {
	return []*store.Node{
		{ID: "hot", Label: "Hot node", Temperature: 0.80},
		{ID: "mid", Label: "Mid node", Temperature: 0.30},
		{ID: "cold", Label: "Cold node", Temperature: 0.05},
		{ID: "pinned", Label: "Pinned cold", Temperature: 0.02, Pinned: true},
	}
}

func eqf(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestNewTemperatureEnvelope_SummaryMirrorsStats(t *testing.T) {
	env := newTemperatureEnvelope(temperatureFixture(), 0.1, 0.5)
	s := env.Summary

	if !eqf(s.Avg, 0.2925) {
		t.Errorf("avg=%v, want 0.2925", s.Avg)
	}
	if !eqf(s.Median, 0.30) {
		t.Errorf("median=%v, want 0.30 (lower-median at offset len/2)", s.Median)
	}

	if s.Hot != 1 {
		t.Errorf("hot=%d, want 1 (temp >= 0.5)", s.Hot)
	}
	if s.Cold != 2 {
		t.Errorf("cold=%d, want 2 (temp < 0.1)", s.Cold)
	}
	if s.Pinned != 1 {
		t.Errorf("pinned=%d, want 1", s.Pinned)
	}

	if !eqf(s.ColdThreshold, 0.1) || !eqf(s.HotThreshold, 0.5) {
		t.Errorf("thresholds echoed wrong: cold=%v hot=%v", s.ColdThreshold, s.HotThreshold)
	}
}

// The configured cold threshold must flow through, not a hardcoded 0.1.
func TestNewTemperatureEnvelope_ColdThresholdIsConfigurable(t *testing.T) {
	env := newTemperatureEnvelope(temperatureFixture(), 0.4, 0.5)

	if env.Summary.Cold != 3 {
		t.Errorf("cold=%d, want 3 (temp < 0.4: 0.02, 0.05, 0.30)", env.Summary.Cold)
	}
	if !eqf(env.Summary.ColdThreshold, 0.4) {
		t.Errorf("cold_threshold=%v, want 0.4 (configured value)", env.Summary.ColdThreshold)
	}
}

// The configured hot threshold must flow through, not a hardcoded 0.5.
func TestNewTemperatureEnvelope_HotThresholdIsConfigurable(t *testing.T) {
	env := newTemperatureEnvelope(temperatureFixture(), 0.1, 0.7)

	// temps: 0.02, 0.05, 0.30, 0.80 → only 0.80 >= 0.7
	if env.Summary.Hot != 1 {
		t.Errorf("hot=%d, want 1 (temp >= 0.7: only 0.80)", env.Summary.Hot)
	}
	if !eqf(env.Summary.HotThreshold, 0.7) {
		t.Errorf("hot_threshold=%v, want 0.7 (configured value)", env.Summary.HotThreshold)
	}
}

func TestNewTemperatureEnvelope_NodesUnifiedAndComplete(t *testing.T) {
	env := newTemperatureEnvelope(temperatureFixture(), 0.1, 0.5)

	if len(env.Nodes) != 4 {
		t.Fatalf("len(nodes)=%d, want 4 (hot, cold, pinned all in one array)", len(env.Nodes))
	}

	byID := make(map[string]tempNode, len(env.Nodes))
	for _, n := range env.Nodes {
		byID[n.ID] = n
	}

	pinned := byID["pinned"]
	if !pinned.Pinned || !eqf(pinned.Temperature, 0.02) || pinned.Label != "Pinned cold" {
		t.Errorf("pinned node mismapped: %+v", pinned)
	}
	hot := byID["hot"]
	if hot.Pinned || !eqf(hot.Temperature, 0.80) || hot.Label != "Hot node" {
		t.Errorf("hot node mismapped: %+v", hot)
	}
}

func TestNewTemperatureEnvelope_Empty(t *testing.T) {
	env := newTemperatureEnvelope(nil, 0.1, 0.5)

	if env.Nodes == nil {
		t.Error("nodes must be non-nil (marshals as [], not null)")
	}
	if len(env.Nodes) != 0 {
		t.Errorf("len(nodes)=%d, want 0", len(env.Nodes))
	}
	if !eqf(env.Summary.Avg, 0) || !eqf(env.Summary.Median, 0) {
		t.Errorf("empty summary must be zero: %+v", env.Summary)
	}
	if !eqf(env.Summary.ColdThreshold, 0.1) || !eqf(env.Summary.HotThreshold, 0.5) {
		t.Errorf("thresholds echoed wrong on empty: cold=%v hot=%v", env.Summary.ColdThreshold, env.Summary.HotThreshold)
	}
}
