package temperature

import (
	"testing"
	"time"

	"github.com/radimsem/remindb/pkg/config"
)

func ptr[T any](v T) *T { return &v }

func TestDefaultConfig_Valid(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("DefaultConfig is invalid: %v", err)
	}
}

func TestValidate_Rejects(t *testing.T) {
	base := DefaultConfig()

	tests := []struct {
		name string
		mut  func(*Config)
	}{
		{"negative DecayRate", func(c *Config) { c.DecayRate = -0.01 }},
		{"negative AccessBoost", func(c *Config) { c.AccessBoost = -0.01 }},
		{"AccessBoost above 1", func(c *Config) { c.AccessBoost = 1.01 }},
		{"negative ColdThreshold", func(c *Config) { c.ColdThreshold = -0.01 }},
		{"ColdThreshold above 1", func(c *Config) { c.ColdThreshold = 1.01 }},
		{"negative NotifyThreshold", func(c *Config) { c.NotifyThreshold = -0.01 }},
		{"NotifyThreshold above 1", func(c *Config) { c.NotifyThreshold = 1.01 }},
		{"negative SummarizeRebound", func(c *Config) { c.SummarizeRebound = -0.01 }},
		{"SummarizeRebound above 1", func(c *Config) { c.SummarizeRebound = 1.01 }},
		{"zero TickInterval", func(c *Config) { c.TickInterval = 0 }},
		{"negative TickInterval", func(c *Config) { c.TickInterval = -time.Second }},
		{"zero ColdNotifyLimit", func(c *Config) { c.ColdNotifyLimit = 0 }},
		{"negative ColdNotifyLimit", func(c *Config) { c.ColdNotifyLimit = -1 }},
		{"negative ColdNotifyTTL", func(c *Config) { c.ColdNotifyTTL = -time.Second }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mut(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("Validate accepted %s", tt.name)
			}
		})
	}
}

func TestValidate_DisabledAllowsZeroTickInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	cfg.TickInterval = 0

	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled config with zero TickInterval rejected: %v", err)
	}
}

func TestNewTracker_RejectsInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DecayRate = -1

	if _, err := NewTracker(nil, "", cfg, nil); err == nil {
		t.Error("NewTracker accepted invalid config")
	}
}

func TestWithOverrides_EmptyKeepsBase(t *testing.T) {
	base := DefaultConfig()

	if got := base.WithOverrides(config.TemperatureConfig{}); got != base {
		t.Errorf("empty overrides changed the config: got %+v, want %+v", got, base)
	}
}

func TestWithOverrides_AllFields(t *testing.T) {
	o := config.TemperatureConfig{
		Enabled:          ptr(false),
		DecayRate:        ptr(0.03),
		AccessBoost:      ptr(0.2),
		ColdThreshold:    ptr(0.08),
		NotifyThreshold:  ptr(0.07),
		SummarizeRebound: ptr(0.6),
		TickInterval:     ptr(config.Duration(10 * time.Minute)),
		ColdNotifyTTL:    ptr(config.Duration(2 * time.Hour)),
		ColdNotifyLimit:  ptr(100),
	}

	got := DefaultConfig().WithOverrides(o)

	want := Config{
		DecayRate:        0.03,
		AccessBoost:      0.2,
		ColdThreshold:    0.08,
		NotifyThreshold:  0.07,
		SummarizeRebound: 0.6,
		TickInterval:     10 * time.Minute,
		ColdNotifyTTL:    2 * time.Hour,
		ColdNotifyLimit:  100,
		Enabled:          false,
	}
	if got != want {
		t.Errorf("WithOverrides = %+v, want %+v", got, want)
	}
}

func TestWithOverrides_PartialKeepsBase(t *testing.T) {
	base := DefaultConfig()

	got := base.WithOverrides(config.TemperatureConfig{DecayRate: ptr(0.99)})

	if got.DecayRate != 0.99 {
		t.Errorf("DecayRate = %g, want 0.99 (overridden)", got.DecayRate)
	}
	if got.AccessBoost != base.AccessBoost {
		t.Errorf("AccessBoost = %g, want %g (untouched default)", got.AccessBoost, base.AccessBoost)
	}
	if got.TickInterval != base.TickInterval {
		t.Errorf("TickInterval = %s, want %s (untouched default)", got.TickInterval, base.TickInterval)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true (untouched default)")
	}
}

// A zero override must overwrite the default — the config fields are pointers.
func TestWithOverrides_ExplicitZeroOverrides(t *testing.T) {
	base := DefaultConfig()

	got := base.WithOverrides(config.TemperatureConfig{DecayRate: ptr(0.0)})

	if got.DecayRate != 0 {
		t.Errorf("DecayRate = %g, want 0 (explicit zero must override default %g)", got.DecayRate, base.DecayRate)
	}
}
