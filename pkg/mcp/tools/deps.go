package tools

import (
	"context"
	"unicode/utf8"

	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type Deps struct {
	Store   *store.Store
	Engine  *query.Engine
	Tracker *temperature.Tracker
}

func (d *Deps) boostResultNodes(ctx context.Context, result *query.Result) {
	if d.Tracker == nil || len(result.Nodes) == 0 {
		return
	}

	ids := make([]string, len(result.Nodes))
	for i, sn := range result.Nodes {
		ids[i] = sn.Node.ID
	}

	_ = d.Tracker.RecordAccess(ctx, ids)
}

func firstLine(s string, maxLen int) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
		if i >= maxLen {
			return s[:maxLen]
		}
	}
	if len(s) > maxLen {
		return s[:maxLen]
	}

	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	end := maxLen
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "..."
}
