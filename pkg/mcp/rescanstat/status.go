// Package rescanstat holds the latest source-rescan tick result for the rescan resource to project.
package rescanstat

import "sync"

type PurgedFile struct {
	Path  string `json:"path"`
	Nodes int    `json:"nodes"`
}

type Snapshot struct {
	RunAt       int64        `json:"run_at"`
	Error       string       `json:"error"`
	Added       int          `json:"added"`
	Modified    int          `json:"modified"`
	Removed     int          `json:"removed"`
	PurgedFiles []PurgedFile `json:"purged_files"`
}

type Status struct {
	mu        sync.Mutex
	intervalS int64
	last      Snapshot
}

func New() *Status { return &Status{} }

// Set publishes one tick's interval and result, replacing the last.
func (s *Status) Set(intervalS int64, snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.intervalS = intervalS
	s.last = snap
}

func (s *Status) Get() (intervalS int64, last Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.intervalS, s.last
}
