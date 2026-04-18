// Package store provides SQLite-backed persistence for nodes, snapshots, and
// diffs.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	db   *sql.DB
	txMu sync.Mutex
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// In-memory databases are per-connection. Limit to one connection so
	// concurrent goroutines share the same schema and data.
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	return &Store{db: db}, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Tx runs fn inside a transaction. Commits on nil error, rolls back otherwise.
func (s *Store) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()

	const maxAttempts = 6
	for attempt := 0; ; attempt++ {
		err := s.runTx(ctx, fn)
		if err == nil {
			return nil
		}

		if attempt >= maxAttempts-1 || !isBusy(err) {
			return err
		}
		if !backoffSleep(ctx, attempt) {
			return ctx.Err()
		}
	}
}

func (s *Store) runTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked")
}

var backoffSteps = []time.Duration{
	10 * time.Millisecond,
	20 * time.Millisecond,
	40 * time.Millisecond,
	80 * time.Millisecond,
	160 * time.Millisecond,
	320 * time.Millisecond,
}

func backoffSleep(ctx context.Context, attempt int) bool {
	d := backoffSteps[len(backoffSteps)-1]
	if attempt < len(backoffSteps) {
		d = backoffSteps[attempt]
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
