package temperature

import "time"

type Config struct {
	DecayRate       float64
	AccessBoost     float64
	ColdThreshold   float64
	NotifyThreshold float64
	TickInterval    time.Duration
}

func DefaultConfig() Config {
	return Config{
		DecayRate:       0.05,
		AccessBoost:     0.15,
		ColdThreshold:   0.1,
		NotifyThreshold: 0.1,
		TickInterval:    5 * time.Minute,
	}
}
