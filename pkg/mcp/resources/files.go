package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/store"
)

const FilesURI = "remindb://files"

type fileEntry struct {
	Path   string `json:"path"`
	Nodes  int    `json:"nodes"`
	Tokens int    `json:"tokens"`
}

type rootGroup struct {
	Root  string      `json:"root"`
	Files []fileEntry `json:"files"`
}

type filesEnvelope struct {
	Roots []rootGroup `json:"roots"`
}

// Group file summaries by compile root into the locked files JSON envelope.
func newFilesEnvelope(summaries []store.FileSummary) filesEnvelope {
	groups := make(map[string][]fileEntry, len(summaries))
	order := make([]string, 0, len(summaries))

	for _, fs := range summaries {
		if _, seen := groups[fs.CompileRoot]; !seen {
			order = append(order, fs.CompileRoot)
		}

		entry := fileEntry{Path: fs.Path, Nodes: fs.NodeCount, Tokens: fs.TokenCount}
		groups[fs.CompileRoot] = append(groups[fs.CompileRoot], entry)
	}

	sort.Slice(order, func(i, j int) bool {
		// Ungrouped bucket (empty CompileRoot) sorts last.
		if order[i] == "" {
			return false
		}
		if order[j] == "" {
			return true
		}

		return order[i] < order[j]
	})

	roots := make([]rootGroup, 0, len(order))
	for _, r := range order {
		roots = append(roots, rootGroup{Root: r, Files: groups[r]})
	}

	return filesEnvelope{Roots: roots}
}

func (d *Deps) HandleFiles(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	summaries, err := d.Store.ListFileSummaries(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list: file summaries: %w", err)
	}

	body, err := json.Marshal(newFilesEnvelope(summaries))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: files: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      FilesURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
