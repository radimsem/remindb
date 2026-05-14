package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/radimsem/remindb/pkg/inspect"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/spf13/cobra"
)

const (
	ansiReset       = "\x1b[0m"
	ansiBold        = "\x1b[1m"
	ansiDim         = "\x1b[2m"
	ansiYellow      = "\x1b[33m"
	ansiCyan        = "\x1b[36m"
	ansiBrightWhite = "\x1b[97m"
)

const (
	defaultInspectDepth = 10
	inspectBranchPad    = 4
	inspectGlyphWidth   = 2
	inspectSubKeyPad    = 14
	inspectLabelPad     = inspectBranchPad + inspectGlyphWidth + 1 + inspectSubKeyPad
	hotThreshold        = 0.5
	coldThreshold       = 0.1
	gradientGreen       = 60
)

var (
	inspectShowTree  bool
	inspectShowFiles bool
	inspectTreeDepth int
	inspectColorOn   bool
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Dump database stats and (optionally) the node tree or file list",
	RunE:  runInspect,
}

func init() {
	inspectCmd.Flags().BoolVar(&inspectShowTree, "tree", false, "Render the node tree")
	inspectCmd.Flags().BoolVar(&inspectShowFiles, "files", false, "Render compiled source files grouped by compile root")
	inspectCmd.Flags().IntVar(&inspectTreeDepth, "depth", defaultInspectDepth, "Maximum tree depth (requires --tree)")
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	if !inspectShowTree && cmd.Flags().Changed("depth") {
		return errors.New("--depth requires --tree")
	}

	inspectColorOn = isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == ""

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	stats, err := inspect.Collect(ctx, st)
	if err != nil {
		return fmt.Errorf("failed to collect stats: %w", err)
	}

	w := os.Stdout
	printStats(w, stats)

	if inspectShowFiles {
		if err := printFilesView(ctx, w, st); err != nil {
			return err
		}
	}
	if !inspectShowTree {
		return nil
	}

	all, err := st.GetAllNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}
	if len(all) == 0 {
		_, _ = fmt.Fprintln(w, "No nodes in database.")
		return nil
	}

	_, _ = fmt.Fprintln(w, paint(ansiBold+ansiCyan, "=== Node Tree ==="))
	roots, childMap := store.BuildTree(all)

	compileRoot, _ := st.GetLatestCompileRoot(ctx)

	for _, root := range roots {
		printTree(w, childMap, root, "", compileRoot, 0, inspectTreeDepth)
	}
	return nil
}

func printFilesView(ctx context.Context, w io.Writer, st *store.Store) error {
	summaries, err := st.ListFileSummaries(ctx)
	if err != nil {
		return fmt.Errorf("failed to list file summaries: %w", err)
	}

	if len(summaries) == 0 {
		_, _ = fmt.Fprintln(w, "No files in database.")
		return nil
	}

	groups, order := groupFilesByRoot(summaries)

	_, _ = fmt.Fprintln(w, paint(ansiBold+ansiCyan, "=== Files ==="))

	for _, root := range order {
		header := root
		if root == "" {
			header = "(ungrouped)"
		}

		_, _ = fmt.Fprintln(w, paint(ansiBold, header))
		renderFileTree(w, groups[root])

		_, _ = fmt.Fprintln(w)
	}
	return nil
}

func groupFilesByRoot(summaries []store.FileSummary) (map[string][]store.FileSummary, []string) {
	groups := make(map[string][]store.FileSummary)
	for _, fs := range summaries {
		groups[fs.CompileRoot] = append(groups[fs.CompileRoot], fs)
	}

	order := make([]string, 0, len(groups))
	for k := range groups {
		order = append(order, k)
	}

	sort.Slice(order, func(i, j int) bool {
		// Ungrouped bucket (empty CompileRoot) renders last.
		if order[i] == "" {
			return false
		}
		if order[j] == "" {
			return true
		}

		return order[i] < order[j]
	})
	return groups, order
}

type fileTrie struct {
	children map[string]*fileTrie
	summary  *store.FileSummary
}

func renderFileTree(w io.Writer, files []store.FileSummary) {
	t := &fileTrie{children: map[string]*fileTrie{}}

	for i := range files {
		segments := strings.Split(files[i].Path, string(filepath.Separator))
		cur := t

		for j, seg := range segments {
			if seg == "" {
				continue
			}

			next, ok := cur.children[seg]
			if !ok {
				next = &fileTrie{children: map[string]*fileTrie{}}
				cur.children[seg] = next
			}

			if j == len(segments)-1 {
				next.summary = &files[i]
			}
			cur = next
		}
	}
	renderTrieNode(w, t, "")
}

func renderTrieNode(w io.Writer, t *fileTrie, prefix string) {
	keys := make([]string, 0, len(t.children))
	for k := range t.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, k := range keys {
		child := t.children[k]
		last := i == len(keys)-1

		branch := "├── "
		nextPrefix := prefix + "│   "
		if last {
			branch = "└── "
			nextPrefix = prefix + "    "
		}

		if child.summary != nil {
			stats := paint(ansiDim, fmt.Sprintf("(%d nodes, %d tok)", child.summary.NodeCount, child.summary.TokenCount))
			_, _ = fmt.Fprintf(w, "%s%s%s %s\n", prefix, branch, paint(ansiBrightWhite, k), stats)
		} else {
			_, _ = fmt.Fprintf(w, "%s%s%s\n", prefix, branch, paint(ansiYellow, k+"/"))
		}

		renderTrieNode(w, child, nextPrefix)
	}
}

type ttyBranch struct {
	key   string
	value string
}

func num(n int) string { return paint(ansiBrightWhite, fmt.Sprintf("%d", n)) }

func num64(n int64) string { return paint(ansiBrightWhite, fmt.Sprintf("%d", n)) }

func printStats(w io.Writer, s *inspect.Stats) {
	header := "=== Database: " + s.DBPath
	if s.DBSizeBytes > 0 {
		header += " (" + inspect.HumanSize(s.DBSizeBytes) + ")"
	}

	header += " ==="
	_, _ = fmt.Fprintln(w, paint(ansiBold+ansiCyan, header))

	nodesValue := fmt.Sprintf("%s (%s tokens)", num(s.NodeCount), num64(s.TokenCountTotal))
	ttyRow(w, "Nodes:", nodesValue)
	ttyBranches(w, mapTTYBranches(s.NodeCountsByType))

	ttyRow(w, "Snapshots:", num(s.SnapshotCount))
	if s.Latest != nil {
		latest := fmt.Sprintf("%s, %s ago",
			paint(ansiBrightWhite, fmt.Sprintf("#%d", s.Latest.ID)),
			paint(ansiBrightWhite, inspect.HumanDuration(s.Latest.AgeSeconds)),
		)
		if s.Latest.Message != "" {
			latest += fmt.Sprintf(", %s", paint(ansiDim, fmt.Sprintf("%q", s.Latest.Message)))
		}

		branches := []ttyBranch{{key: "latest:", value: latest}}
		if s.Latest.CursorHash != "" {
			branches = append(branches, ttyBranch{key: "cursor:", value: paint(ansiDim, inspect.TruncateHash(s.Latest.CursorHash))})
		}

		ttyBranches(w, branches)
	}

	ttyRow(w, "Temperature:", "")
	tempBranches := []ttyBranch{
		{key: "avg:", value: tempPaint(s.AvgTemp)},
		{key: "median:", value: tempPaint(s.MedianTemp)},
		{key: fmt.Sprintf("hot (≥%.1f):", hotThreshold), value: num(s.HotCount)},
		{key: fmt.Sprintf("cold (<%.1f):", coldThreshold), value: num(s.ColdCount)},
		{key: "pinned:", value: num(s.PinnedCount)},
	}

	ttyBranches(w, tempBranches)
	ttyRow(w, "Relations:", num(s.RelationCount))

	relBranches := mapTTYBranches(s.RelationsByOrigin)
	if s.PendingRelationCount > 0 {
		relBranches = append(relBranches, ttyBranch{key: "pending:", value: num(s.PendingRelationCount)})
	}

	ttyBranches(w, relBranches)
	ttyRow(w, "FTS rows:", num(s.FTSRowCount))

	_, _ = fmt.Fprintln(w)
}

func ttyRow(w io.Writer, label, value string) {
	padded := inspect.RunePad(label, inspectLabelPad)

	if value == "" {
		_, _ = fmt.Fprintln(w, paint(ansiDim, padded))
		return
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", paint(ansiDim, padded), value)
}

func ttyBranches(w io.Writer, branches []ttyBranch) {
	prefix := strings.Repeat(" ", inspectBranchPad)

	for i, br := range branches {
		glyph := "├─"
		if i == len(branches)-1 {
			glyph = "└─"
		}

		key := paint(ansiDim, inspect.RunePad(br.key, inspectSubKeyPad))
		_, _ = fmt.Fprintf(w, "%s%s %s %s\n", prefix, paint(ansiDim, glyph), key, br.value)
	}
}

func mapTTYBranches(m map[string]int) []ttyBranch {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	out := make([]ttyBranch, 0, len(keys))
	for _, k := range keys {
		out = append(out, ttyBranch{key: k + ":", value: num(m[k])})
	}

	return out
}

func printTree(w io.Writer, children map[string][]*store.Node, n *store.Node, parentSource, compileRoot string, depth, maxDepth int) {
	indent := strings.Repeat("  ", depth)

	_, _ = fmt.Fprintf(w, "%s%s %s (%s",
		indent,
		paint(ansiYellow, "["+n.NodeType+"]"),
		paint(ansiBrightWhite, n.Label),
		paint(ansiDim, "id="+n.ID),
	)

	if n.SourceFile != parentSource {
		_, _ = fmt.Fprintf(w, " %s", paint(ansiDim, "file="+relSourcePath(n.SourceFile, compileRoot)))
	}

	_, _ = fmt.Fprintf(w, " %s %s)\n",
		"temp="+tempPaint(n.Temperature),
		paint(ansiDim, fmt.Sprintf("tok=%d", n.TokenCount)),
	)

	if depth >= maxDepth {
		return
	}

	for _, child := range children[n.ID] {
		printTree(w, children, child, n.SourceFile, compileRoot, depth+1, maxDepth)
	}
}

func relSourcePath(source, compileRoot string) string {
	if compileRoot == "" || !filepath.IsAbs(source) {
		return source
	}

	rel, err := filepath.Rel(compileRoot, source)
	if err != nil || strings.HasPrefix(rel, "..") {
		return source
	}

	return rel
}

func paint(code, s string) string {
	if !inspectColorOn {
		return s
	}
	return code + s + ansiReset
}

// Gradient blue (cold) → red (hot) over [0,1].
func tempPaint(t float64) string {
	s := fmt.Sprintf("%.2f", t)
	if !inspectColorOn {
		return s
	}

	c := t
	if c < 0 {
		c = 0
	}
	if c > 1 {
		c = 1
	}
	r := int(255 * c)
	g := gradientGreen
	b := int(255 * (1 - c))

	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s%s", r, g, b, s, ansiReset)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
