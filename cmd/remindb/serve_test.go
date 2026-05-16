package main

import (
	"testing"
	"time"

	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/temperature"
)

func ptr[T any](v T) *T { return &v }

func TestApplyTemperatureOverrides_Empty(t *testing.T) {
	base := temperature.DefaultConfig()

	got := applyTemperatureOverrides(base, config.TemperatureConfig{})
	if got != base {
		t.Errorf("empty overrides changed the config: got %+v, want %+v", got, base)
	}
}

func TestApplyTemperatureOverrides_AllFields(t *testing.T) {
	base := temperature.DefaultConfig()

	o := config.TemperatureConfig{
		DecayRate:        ptr(0.03),
		AccessBoost:      ptr(0.2),
		ColdThreshold:    ptr(0.08),
		NotifyThreshold:  ptr(0.07),
		SummarizeRebound: ptr(0.6),
		TickInterval:     ptr(config.Duration(10 * time.Minute)),
		ColdNotifyTTL:    ptr(config.Duration(2 * time.Hour)),
		ColdNotifyLimit:  ptr(100),
	}

	got := applyTemperatureOverrides(base, o)

	want := temperature.Config{
		DecayRate:        0.03,
		AccessBoost:      0.2,
		ColdThreshold:    0.08,
		NotifyThreshold:  0.07,
		SummarizeRebound: 0.6,
		TickInterval:     10 * time.Minute,
		ColdNotifyTTL:    2 * time.Hour,
		ColdNotifyLimit:  100,
	}
	if got != want {
		t.Errorf("applyTemperatureOverrides = %+v, want %+v", got, want)
	}
}

func TestApplyTemperatureOverrides_PartialKeepsBase(t *testing.T) {
	base := temperature.DefaultConfig()

	got := applyTemperatureOverrides(base, config.TemperatureConfig{DecayRate: ptr(0.99)})

	if got.DecayRate != 0.99 {
		t.Errorf("DecayRate = %g, want 0.99 (overridden)", got.DecayRate)
	}
	if got.AccessBoost != base.AccessBoost {
		t.Errorf("AccessBoost = %g, want %g (untouched default)", got.AccessBoost, base.AccessBoost)
	}
	if got.TickInterval != base.TickInterval {
		t.Errorf("TickInterval = %s, want %s (untouched default)", got.TickInterval, base.TickInterval)
	}
}

// A zero override must overwrite the default — the reason fields are pointers.
func TestApplyTemperatureOverrides_ExplicitZeroOverrides(t *testing.T) {
	base := temperature.DefaultConfig()

	got := applyTemperatureOverrides(base, config.TemperatureConfig{DecayRate: ptr(0.0)})

	if got.DecayRate != 0 {
		t.Errorf("DecayRate = %g, want 0 (explicit zero must override default %g)", got.DecayRate, base.DecayRate)
	}
}
