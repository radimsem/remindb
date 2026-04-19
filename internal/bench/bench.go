package bench

import (
	"context"
	"fmt"
	"io"
)

type Config struct {
	DBPath  string
	Dir     string
	Budget  int
	Queries []string
	Out     io.Writer
	Stderr  io.Writer
}

func Run(ctx context.Context, cfg Config) error {
	stage, err := stageBench(ctx, cfg.DBPath, cfg.Dir)
	if err != nil {
		return err
	}
	defer stage.cleanup()

	session, err := spawnServerClient(ctx, stage.dbPath, cfg.Stderr)
	if err != nil {
		return fmt.Errorf("failed to start: bench server: %w", err)
	}
	defer func() { _ = session.Close() }()

	var results []scenarioResult

	r, err := benchTree(ctx, session, stage.srcDir)
	if err != nil {
		return err
	}
	results = append(results, r)

	if len(cfg.Queries) > 0 {
		rs, err := benchSearch(ctx, session, stage.srcDir, cfg.Queries, cfg.Budget)
		if err != nil {
			return err
		}
		results = append(results, rs...)
	}

	r, err = benchFetch(ctx, session, stage.srcDir, stage.dbPath, cfg.Budget)
	if err != nil {
		return err
	}
	results = append(results, r)

	r, err = benchDelta(ctx, session, stage.srcDir, stage.dbPath)
	if err != nil {
		return err
	}
	results = append(results, r)

	return renderResults(cfg.Out, results)
}

// One row of the output table.
type scenarioResult struct {
	name       string
	naiveTok   int
	remindbTok int
}
