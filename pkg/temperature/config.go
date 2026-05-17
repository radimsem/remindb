package temperature

import (
	"fmt"
	"time"

	"github.com/radimsem/remindb/pkg/config"
)

type Config struct {
	DecayRate        float64
	AccessBoost      float64
	ColdThreshold    float64
	NotifyThreshold  float64
	SummarizeRebound float64
	TickInterval     time.Duration
	ColdNotifyTTL    time.Duration
	ColdNotifyLimit  int
	Enabled          bool
}

func DefaultConfig() Config {
	return Config{
		DecayRate:        0.05,
		AccessBoost:      0.15,
		ColdThreshold:    0.1,
		NotifyThreshold:  0.1,
		SummarizeRebound: 0.5,
		TickInterval:     5 * time.Minute,
		ColdNotifyTTL:    time.Hour,
		ColdNotifyLimit:  50,
		Enabled:          true,
	}
}

// Apply a config.json temperature block over c; nil fields keep c's value.
func (c Config) WithOverrides(o config.TemperatureConfig) Config {
	if o.Enabled != nil {
		c.Enabled = *o.Enabled
	}
	if o.DecayRate != nil {
		c.DecayRate = *o.DecayRate
	}
	if o.AccessBoost != nil {
		c.AccessBoost = *o.AccessBoost
	}
	if o.ColdThreshold != nil {
		c.ColdThreshold = *o.ColdThreshold
	}
	if o.NotifyThreshold != nil {
		c.NotifyThreshold = *o.NotifyThreshold
	}
	if o.SummarizeRebound != nil {
		c.SummarizeRebound = *o.SummarizeRebound
	}
	if o.TickInterval != nil {
		c.TickInterval = time.Duration(*o.TickInterval)
	}
	if o.ColdNotifyTTL != nil {
		c.ColdNotifyTTL = time.Duration(*o.ColdNotifyTTL)
	}
	if o.ColdNotifyLimit != nil {
		c.ColdNotifyLimit = *o.ColdNotifyLimit
	}
	return c
}

func inUnit(v float64) bool {
	return v >= 0 && v <= 1
}

func (c Config) Validate() error {
	if c.DecayRate < 0 {
		return fmt.Errorf("DecayRate must be >= 0, got %g", c.DecayRate)
	}

	if !inUnit(c.AccessBoost) {
		return fmt.Errorf("AccessBoost must be in [0, 1], got %g", c.AccessBoost)
	}
	if !inUnit(c.ColdThreshold) {
		return fmt.Errorf("ColdThreshold must be in [0, 1], got %g", c.ColdThreshold)
	}
	if !inUnit(c.NotifyThreshold) {
		return fmt.Errorf("NotifyThreshold must be in [0, 1], got %g", c.NotifyThreshold)
	}
	if !inUnit(c.SummarizeRebound) {
		return fmt.Errorf("SummarizeRebound must be in [0, 1], got %g", c.SummarizeRebound)
	}

	if c.Enabled && c.TickInterval <= 0 {
		return fmt.Errorf("TickInterval must be > 0, got %s", c.TickInterval)
	}
	if c.ColdNotifyLimit <= 0 {
		return fmt.Errorf("ColdNotifyLimit must be > 0, got %d", c.ColdNotifyLimit)
	}
	if c.ColdNotifyTTL < 0 {
		return fmt.Errorf("ColdNotifyTTL must be >= 0, got %s", c.ColdNotifyTTL)
	}
	return nil
}
