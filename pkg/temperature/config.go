package temperature

import (
	"fmt"
	"time"
)

type Config struct {
	DecayRate       float64
	AccessBoost     float64
	ColdThreshold   float64
	NotifyThreshold float64
	TickInterval    time.Duration
	ColdNotifyLimit int
}

func DefaultConfig() Config {
	return Config{
		DecayRate:       0.05,
		AccessBoost:     0.15,
		ColdThreshold:   0.1,
		NotifyThreshold: 0.1,
		TickInterval:    5 * time.Minute,
		ColdNotifyLimit: 50,
	}
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

	if c.TickInterval <= 0 {
		return fmt.Errorf("TickInterval must be > 0, got %s", c.TickInterval)
	}
	if c.ColdNotifyLimit <= 0 {
		return fmt.Errorf("ColdNotifyLimit must be > 0, got %d", c.ColdNotifyLimit)
	}
	return nil
}
