package tools

import (
	"context"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type Deps struct {
	Store   *store.Store
	Engine  *query.Engine
	Tracker *temperature.Tracker
	Logger  *slog.Logger
}

func (d *Deps) logCall(name string, errp *error, start time.Time, attrs ...any) {
	if d.Logger == nil {
		return
	}

	fields := []any{"tool", name, "elapsed_ms", time.Since(start).Milliseconds()}
	fields = append(fields, attrs...)
	if *errp != nil {
		fields = append(fields, "err", *errp)
		d.Logger.Error("mcp call failed", fields...)
		return
	}
	d.Logger.Debug("mcp call", fields...)
}

func (d *Deps) boostResultNodes(ctx context.Context, result *query.Result) {
	if d.Tracker == nil || len(result.Nodes) == 0 {
		return
	}

	ids := make([]string, len(result.Nodes))
	for i, sn := range result.Nodes {
		ids[i] = sn.Node.ID
	}

	if err := d.Tracker.RecordAccess(ctx, ids); err != nil && d.Logger != nil {
		d.Logger.Warn("failed to boost: access", "err", err, "count", len(ids), "ids", ids)
	}
}

// Emit one snapshot for a single mutated or newly created node.
func emitNodeChange(ctx context.Context, st *store.Store, node *parser.ContextNode, prev map[string]diff.NodeState, msg string) error {
	roots := []*parser.ContextNode{node}
	return emitter.Emit(ctx, st,
		emitter.WithRoots(roots),
		emitter.WithDeltas(diff.Diff(roots, prev)),
		emitter.WithCursorHash(diff.CursorHash(roots)),
		emitter.WithMessage(msg),
	)
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
