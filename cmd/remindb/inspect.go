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
	"unicode/utf8"

	"github.com/radimsem/remindb/pkg/store"
	"github.com/spf13/cobra"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiYellow  = "\x1b[33m"
	ansiCyan    = "\x1b[36m"
	ansiBrightW = "\x1b[97m"
)

const (
	defaultInspectDepth = 10
	inspectLabelPad     = 14
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

	stats, err := st.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
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
			_, _ = fmt.Fprintf(w, "%s%s%s %s\n", prefix, branch, paint(ansiBrightW, k), stats)
		} else {
			_, _ = fmt.Fprintf(w, "%s%s%s\n", prefix, branch, paint(ansiYellow, k+"/"))
		}

		renderTrieNode(w, child, nextPrefix)
	}
}

func printStats(w io.Writer, s *store.Stats) {
	header := "=== Database: " + dbPath
	if fi, err := os.Stat(dbPath); err == nil {
		header += " (" + humanSize(fi.Size()) + ")"
	}

	header += " ==="
	_, _ = fmt.Fprintln(w, paint(ansiBold+ansiCyan, header))

	row := func(label, value string) {
		padded := runePad(label, inspectLabelPad)
		_, _ = fmt.Fprintf(w, "%s %s\n", paint(ansiDim, padded), value)
	}
	num := func(n int) string { return paint(ansiBrightW, fmt.Sprintf("%d", n)) }

	hotLabel := fmt.Sprintf("Hot (≥%.1f):", hotThreshold)
	coldLabel := fmt.Sprintf("Cold (<%.1f):", coldThreshold)

	row("Nodes:", num(s.NodeCount))
	row("Snapshots:", num(s.SnapshotCount))
	row("Avg temp:", tempPaint(s.AvgTemp))
	row(hotLabel, num(s.HotCount))
	row(coldLabel, num(s.ColdCount))
	_, _ = fmt.Fprintln(w)
}

func printTree(w io.Writer, children map[string][]*store.Node, n *store.Node, parentSource, compileRoot string, depth, maxDepth int) {
	indent := strings.Repeat("  ", depth)

	_, _ = fmt.Fprintf(w, "%s%s %s (%s",
		indent,
		paint(ansiYellow, "["+n.NodeType+"]"),
		paint(ansiBrightW, n.Label),
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

func runePad(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0

	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
