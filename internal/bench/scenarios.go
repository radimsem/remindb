package bench

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/tokens"
	"github.com/radimsem/remindb/pkg/store"
)

// Tree: MemoryTree vs `find` + `cat *` over the dir.
func benchTree(ctx context.Context, s *gomcp.ClientSession, srcDir string) (scenarioResult, error) {
	naive := tokens.Estimate(listDirFiles(srcDir)) + countDirTokens(srcDir)

	text, err := callTool(ctx, s, "MemoryTree", map[string]any{})
	if err != nil {
		return scenarioResult{}, err
	}
	return scenarioResult{"tree", naive, tokens.Estimate(text)}, nil
}

// Search: MemorySearch + MemoryFetch vs `grep` + `cat <matches>` over the matched files from the grep result.
func benchSearch(ctx context.Context, s *gomcp.ClientSession, srcDir string, queries []string, budget int) ([]scenarioResult, error) {
	out := make([]scenarioResult, 0, len(queries))

	for _, q := range queries {
		grepOut, matched := grepDir(srcDir, strings.Fields(q))
		naive := tokens.Estimate(grepOut) + sumFileTokens(matched)

		searchText, err := callTool(ctx, s, "MemorySearch", map[string]any{
			"query":  q,
			"budget": budget,
		})
		if err != nil {
			return nil, err
		}
		remindbTok := tokens.Estimate(searchText)

		if topID := parseTopNodeID(searchText); topID != "" {
			fetchText, err := callTool(ctx, s, "MemoryFetch", map[string]any{
				"anchor": topID,
				"budget": budget,
			})
			if err != nil {
				return nil, err
			}

			remindbTok += tokens.Estimate(fetchText)
		}

		out = append(out, scenarioResult{
			name:       "search:" + shorten(q, 30),
			naiveTok:   naive,
			remindbTok: remindbTok,
		})
	}
	return out, nil
}

// Fetch: budget-bounded context around a mid-sized anchor, vs reading the whole source file that contains it.
func benchFetch(ctx context.Context, s *gomcp.ClientSession, srcDir, dbPath string, budget int) (scenarioResult, error) {
	anchor, err := pickMidsizeNode(ctx, dbPath)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("failed to pick: fetch anchor: %w", err)
	}

	sourcePath := filepath.Join(srcDir, anchor.SourceFile)
	data, err := os.ReadFile(sourcePath)

	if err != nil {
		return scenarioResult{}, fmt.Errorf("failed to read: %s: %w", sourcePath, err)
	}
	naive := tokens.Estimate(string(data))

	fetchText, err := callTool(ctx, s, "MemoryFetch", map[string]any{
		"anchor": anchor.ID,
		"budget": budget,
	})
	if err != nil {
		return scenarioResult{}, err
	}

	return scenarioResult{"fetch", naive, tokens.Estimate(fetchText)}, nil
}

// Delta: MemoryDelta vs `diff -u` + read the context around the change over the modified file.
func benchDelta(ctx context.Context, s *gomcp.ClientSession, srcDir, dbPath string) (scenarioResult, error) {
	target, err := pickFirstMarkdownFile(srcDir)
	if err != nil {
		return scenarioResult{}, err
	}

	original, err := os.ReadFile(target)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("failed to read: %s: %w", target, err)
	}

	// Save the untouched original alongside the modified file so `diff -u` has a baseline to compare against.
	snapshot, err := os.CreateTemp("", "remindb-bench-orig-*")
	if err != nil {
		return scenarioResult{}, fmt.Errorf("failed to create: original snapshot: %w", err)
	}

	snapshotPath := snapshot.Name()
	defer func() { _ = os.Remove(snapshotPath) }()

	if _, err := snapshot.Write(original); err != nil {
		_ = snapshot.Close()
		return scenarioResult{}, fmt.Errorf("failed to write: snapshot: %w", err)
	}
	_ = snapshot.Close()

	modified := append(original, []byte(deltaPatch)...)
	if err := os.WriteFile(target, modified, 0o644); err != nil {
		return scenarioResult{}, fmt.Errorf("failed to write: modified %s: %w", target, err)
	}

	diffOut, err := diffUnified(snapshotPath, target)
	if err != nil {
		return scenarioResult{}, err
	}
	naive := tokens.Estimate(diffOut) + tokens.Estimate(string(modified))

	baselineSnapID, err := latestSnapshotID(ctx, dbPath)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("failed to read: baseline snapshot id: %w", err)
	}

	compileText, err := callTool(ctx, s, "MemoryCompile", map[string]any{
		"path":    srcDir,
		"message": "bench-delta",
	})
	if err != nil {
		return scenarioResult{}, err
	}

	deltaText, err := callTool(ctx, s, "MemoryDelta", map[string]any{
		"since_snapshot": baselineSnapID,
	})
	if err != nil {
		return scenarioResult{}, err
	}
	remindbTok := tokens.Estimate(compileText) + tokens.Estimate(deltaText)

	return scenarioResult{"delta", naive, remindbTok}, nil
}

const deltaPatch = "\n\n## Bench synthetic section\n\nThis paragraph is appended by `remindb bench` to measure delta tokens. It " +
	"introduces fresh content so MemoryDelta has a real change to describe " +
	"rather than returning an empty diff.\n"

// Mirror the naive `find` or `ls -R` output an agent would emit.
func listDirFiles(dir string) string {
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !fileext.Supported(path) {
			return err
		}

		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(&b, "./%s\n", rel)
		return nil
	})
	return b.String()
}

func countDirTokens(dir string) int {
	total := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !fileext.Supported(path) {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		total += tokens.Estimate(string(data))
		return nil
	})
	return total
}

// Match the output shape of `grep -rn <terms>`.
func grepDir(dir string, terms []string) (string, []string) {
	var b strings.Builder
	seen := make(map[string]bool)
	var matched []string

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !fileext.Supported(path) {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		for i, line := range strings.Split(string(data), "\n") {
			lower := strings.ToLower(line)

			for _, term := range terms {
				if strings.Contains(lower, strings.ToLower(term)) {
					fmt.Fprintf(&b, "%s:%d:%s\n", rel, i+1, line)

					if !seen[path] {
						seen[path] = true
						matched = append(matched, path)
					}
					break
				}
			}
		}
		return nil
	})
	return b.String(), matched
}

func sumFileTokens(files []string) int {
	total := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		total += tokens.Estimate(string(data))
	}
	return total
}

func parseTopNodeID(text string) string {
	_, rest, ok := strings.Cut(text, "(id=")
	if !ok {
		return ""
	}

	if j := strings.IndexAny(rest, " )"); j > 0 {
		return rest[:j]
	}
	return ""
}

// Pick the largest source file by total node token count, then return a depth >= 2
// node from the 25-50th percentile band of TokenCount within that file.
func pickMidsizeNode(ctx context.Context, dbPath string) (*store.Node, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = st.Close() }()

	all, err := st.GetAllNodes(ctx)
	if err != nil {
		return nil, err
	}

	fileTokens := make(map[string]int)
	fileCandidates := make(map[string][]*store.Node)
	for _, n := range all {
		fileTokens[n.SourceFile] += n.TokenCount
		if n.Depth < 2 || n.TokenCount == 0 {
			continue
		}

		fileCandidates[n.SourceFile] = append(fileCandidates[n.SourceFile], n)
	}

	var largestFile string
	largestTotal := 0
	for f, tot := range fileTokens {
		if len(fileCandidates[f]) == 0 {
			continue
		}

		if tot > largestTotal {
			largestTotal = tot
			largestFile = f
		}
	}

	candidates := fileCandidates[largestFile]
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no depth >= 2 nodes with content found; compile more content first")
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TokenCount < candidates[j].TokenCount
	})

	lo := len(candidates) / 4
	hi := len(candidates) / 2
	if hi <= lo {
		hi = lo + 1
	}

	return candidates[(lo+hi)/2], nil
}

func pickFirstMarkdownFile(dir string) (string, error) {
	var hit string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}
		hit = path
		return filepath.SkipAll
	})

	if err != nil {
		return "", fmt.Errorf("failed to walk: %s: %w", dir, err)
	}
	if hit == "" {
		return "", fmt.Errorf("no markdown files under %s for delta scenario", dir)
	}

	return hit, nil
}

func latestSnapshotID(ctx context.Context, dbPath string) (int64, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = st.Close() }()

	snaps, err := st.ListSnapshots(ctx, 1)
	if err != nil {
		return 0, err
	}

	if len(snaps) == 0 {
		return 0, nil
	}
	return snaps[0].ID, nil
}

func diffUnified(a, b string) (string, error) {
	cmd := exec.Command("diff", "-u", a, b)
	out, err := cmd.Output()

	// `diff -u` exits with `1` on success.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return string(out), nil
	}

	if err != nil {
		return "", fmt.Errorf("failed to diff: %w", err)
	}
	return string(out), nil
}

func shorten(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
