package temperature

import (
	"testing"
	"time"
)

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

func TestNewTracker_RejectsInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DecayRate = -1

	if _, err := NewTracker(nil, cfg, nil); err == nil {
		t.Error("NewTracker accepted invalid config")
	}
}
