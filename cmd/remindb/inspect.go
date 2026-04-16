package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/radimsem/remindb/pkg/store"
	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Dump database stats, node tree, and temperature map",
	RunE:  runInspect,
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(_ *cobra.Command, _ []string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	stats, err := st.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "=== Database: %s ===\n", dbPath)
	_, _ = fmt.Fprintf(os.Stdout, "Nodes:     %d\n", stats.NodeCount)
	_, _ = fmt.Fprintf(os.Stdout, "Snapshots: %d\n", stats.SnapshotCount)
	_, _ = fmt.Fprintf(os.Stdout, "Avg temp:  %.3f\n", stats.AvgTemp)
	_, _ = fmt.Fprintf(os.Stdout, "Hot (≥0.5): %d\n", stats.HotCount)
	_, _ = fmt.Fprintf(os.Stdout, "Cold (<0.1): %d\n\n", stats.ColdCount)

	roots, err := st.GetRootNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get roots: %w", err)
	}

	if len(roots) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No nodes in database.")
		return nil
	}

	const maxTreeDepth = 10

	_, _ = fmt.Fprintln(os.Stdout, "=== Node Tree ===")
	for _, root := range roots {
		if err := printTree(ctx, st, root, 0, maxTreeDepth); err != nil {
			return err
		}
	}
	return nil
}

func printTree(ctx context.Context, st *store.Store, n *store.Node, depth, maxDepth int) error {
	indent := strings.Repeat("  ", depth)
	_, _ = fmt.Fprintf(os.Stdout, "%s[%s] %s (id=%s temp=%.2f tok=%d)\n",
		indent, n.NodeType, n.Label, n.ID, n.Temperature, n.TokenCount)

	if depth >= maxDepth {
		return nil
	}

	children, err := st.GetChildren(ctx, n.ID)
	if err != nil {
		return fmt.Errorf("failed to get children: %s: %w", n.ID, err)
	}

	for _, child := range children {
		if err := printTree(ctx, st, child, depth+1, maxDepth); err != nil {
			return err
		}
	}
	return nil
}
