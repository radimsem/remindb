package mcp

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/store"
)

const defaultRescanInterval = 30 * time.Second

type RescanLoop struct {
	store    *store.Store
	dir      string
	interval time.Duration
	modTimes map[string]time.Time
}

func NewRescanLoop(st *store.Store, dir string, interval time.Duration) *RescanLoop {
	if interval <= 0 {
		interval = defaultRescanInterval
	}
	return &RescanLoop{
		store:    st,
		dir:      dir,
		interval: interval,
		modTimes: make(map[string]time.Time),
	}
}

func (r *RescanLoop) Run(ctx context.Context) {
	// Avoid recompiling all files on the first tick.
	r.seedMtimes()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.scan(ctx)
		}
	}
}

func (r *RescanLoop) seedMtimes() {
	_ = filepath.WalkDir(r.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !fileext.Supported(path) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		r.modTimes[path] = info.ModTime()
		return nil
	})
}

func (r *RescanLoop) scan(ctx context.Context) {
	var changed []string
	seen := make(map[string]bool, len(r.modTimes))

	_ = filepath.WalkDir(r.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !fileext.Supported(path) {
			return nil
		}

		seen[path] = true

		info, err := d.Info()
		if err != nil {
			return nil
		}

		prev, ok := r.modTimes[path]
		if !ok || info.ModTime().After(prev) {
			changed = append(changed, path)
			r.modTimes[path] = info.ModTime()
		}
		return nil
	})

	// Purge entries for deleted files.
	for path := range r.modTimes {
		if !seen[path] {
			delete(r.modTimes, path)
		}
	}

	if len(changed) == 0 {
		return
	}

	result, err := compiler.Compile(ctx, r.store, changed, "rescan", nil)
	if err != nil {
		log.Printf("rescan: compile error: %v", err)
		return
	}

	log.Printf("rescan: %d added, %d modified, %d removed (%d total)",
		result.Added, result.Modified, result.Removed, result.Total)
}
