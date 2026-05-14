package inspect

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	hotThreshold  = 0.5
	coldThreshold = 0.1

	branchPad  = 4
	glyphWidth = 2
	subKeyPad  = 14
	labelPad   = branchPad + glyphWidth + 1 + subKeyPad
	maxHashLen = 16
)

type branch struct {
	key   string
	value string
}

// Render the stats as a plain text block.
func Format(s *Stats) string {
	var b strings.Builder

	header := "Database: " + s.DBPath
	if s.DBSizeBytes > 0 {
		header += " (" + HumanSize(s.DBSizeBytes) + ")"
	}
	fmt.Fprintln(&b, header)

	writeNodes(&b, s)
	writeSnapshots(&b, s)
	writeTemperature(&b, s)
	writeRelations(&b, s)

	row(&b, "FTS rows:", fmt.Sprintf("%d", s.FTSRowCount))
	return b.String()
}

func writeNodes(b *strings.Builder, s *Stats) {
	total := fmt.Sprintf("%d (%d tokens)", s.NodeCount, s.TokenCountTotal)
	row(b, "Nodes:", total)
	writeBranches(b, mapBranches(s.NodeCountsByType))
}

func writeSnapshots(b *strings.Builder, s *Stats) {
	row(b, "Snapshots:", fmt.Sprintf("%d", s.SnapshotCount))
	if s.Latest == nil {
		return
	}

	latest := fmt.Sprintf("#%d, %s ago", s.Latest.ID, HumanDuration(s.Latest.AgeSeconds))
	if s.Latest.Message != "" {
		latest += fmt.Sprintf(", %q", s.Latest.Message)
	}

	branches := []branch{{key: "latest:", value: latest}}
	if s.Latest.CursorHash != "" {
		branches = append(branches, branch{key: "cursor:", value: TruncateHash(s.Latest.CursorHash)})
	}
	writeBranches(b, branches)
}

func writeTemperature(b *strings.Builder, s *Stats) {
	row(b, "Temperature:", "")
	branches := []branch{
		{key: "avg:", value: fmt.Sprintf("%.2f", s.AvgTemp)},
		{key: "median:", value: fmt.Sprintf("%.2f", s.MedianTemp)},
		{key: fmt.Sprintf("hot (>=%.1f):", hotThreshold), value: fmt.Sprintf("%d", s.HotCount)},
		{key: fmt.Sprintf("cold (<%.1f):", coldThreshold), value: fmt.Sprintf("%d", s.ColdCount)},
		{key: "pinned:", value: fmt.Sprintf("%d", s.PinnedCount)},
	}
	writeBranches(b, branches)
}

func writeRelations(b *strings.Builder, s *Stats) {
	row(b, "Relations:", fmt.Sprintf("%d", s.RelationCount))

	branches := mapBranches(s.RelationsByOrigin)
	if s.PendingRelationCount > 0 {
		branches = append(branches, branch{key: "pending:", value: fmt.Sprintf("%d", s.PendingRelationCount)})
	}
	writeBranches(b, branches)
}

func mapBranches(m map[string]int) []branch {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]branch, 0, len(keys))
	for _, k := range keys {
		out = append(out, branch{key: k + ":", value: fmt.Sprintf("%d", m[k])})
	}
	return out
}

func writeBranches(b *strings.Builder, branches []branch) {
	prefix := strings.Repeat(" ", branchPad)

	for i, br := range branches {
		glyph := "├─"
		if i == len(branches)-1 {
			glyph = "└─"
		}

		fmt.Fprintf(b, "%s%s %s %s\n", prefix, glyph, RunePad(br.key, subKeyPad), br.value)
	}
}

func row(b *strings.Builder, label, value string) {
	if value == "" {
		fmt.Fprintln(b, RunePad(label, labelPad))
		return
	}
	fmt.Fprintf(b, "%s %s\n", RunePad(label, labelPad), value)
}

// Pad s to width visible runes, appending spaces on the right.
func RunePad(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}

	return s + strings.Repeat(" ", width-n)
}

// Truncate an opaque hash to maxHashLen runes, suffixing "..." if shortened.
func TruncateHash(h string) string {
	if len(h) <= maxHashLen {
		return h
	}
	return h[:maxHashLen] + "..."
}

// Render n as a human-readable size (B / KB / MB / …).
func HumanSize(n int64) string {
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

// Render a non-negative duration in seconds as the largest non-zero unit (s/m/h/d).
func HumanDuration(seconds int64) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh", seconds/3600)
	default:
		return fmt.Sprintf("%dd", seconds/86400)
	}
}
