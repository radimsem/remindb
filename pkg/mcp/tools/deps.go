package tools

import (
	"context"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/radimsem/remindb/internal/redaction"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/mcp/notify"
	"github.com/radimsem/remindb/pkg/mcp/resources"
	"github.com/radimsem/remindb/pkg/mcp/sessionlog"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/relations"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type Deps struct {
	Store            *store.Store
	Engine           *query.Engine
	Resolver         *relations.Resolver
	Tracker          *temperature.Tracker
	Redactor         *redaction.Redactor
	Logger           *slog.Logger
	SourceDir        string
	WorkspaceConfig  config.Config
	SummarizeRebound float64
	Notifier         *notify.Publisher
}

// touchSnapshot signals the resources a node-graph snapshot mutates.
func (d *Deps) touchSnapshot() {
	if d.Notifier == nil {
		return
	}

	d.Notifier.Touch(resources.SnapshotsURI)
	d.Notifier.Touch(resources.TreeURI)
	d.Notifier.Touch(resources.GraphURI)
}

// touchCompile is touchSnapshot plus the file set, which only compile reshapes.
func (d *Deps) touchCompile() {
	if d.Notifier == nil {
		return
	}

	d.touchSnapshot()
	d.Notifier.Touch(resources.FilesURI)
}

func (d *Deps) touchGraph() {
	if d.Notifier == nil {
		return
	}

	d.Notifier.Touch(resources.GraphURI)
}

func (d *Deps) logCall(ctx context.Context, name string, errp *error, start time.Time, attrs ...any) {
	if d.Logger == nil {
		return
	}

	fields := []any{"tool", name, "elapsed_ms", time.Since(start).Milliseconds()}
	fields = append(fields, attrs...)

	if *errp != nil {
		fields = append(fields, "err", *errp)
		d.Logger.ErrorContext(ctx, sessionlog.MsgToolCallFailed, fields...)
		return
	}
	d.Logger.DebugContext(ctx, sessionlog.MsgToolCall, fields...)
}

func (d *Deps) logRedaction(source string, hits []redaction.Hit) {
	if d.Logger == nil || len(hits) == 0 {
		return
	}

	d.Logger.Info("redacted secrets",
		"source", source,
		"hit_count", len(hits),
		"kinds", redaction.KindCounts(hits))
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
		d.Logger.WarnContext(ctx, "failed to boost: access", "err", err, "count", len(ids), "ids", ids)
	}
}

// Emit one snapshot for a single mutated or newly created node, then signal the resources that snapshot reshaped.
func (d *Deps) emitNodeChange(ctx context.Context, node *parser.ContextNode, prev map[string]diff.NodeState, msg string) error {
	roots := []*parser.ContextNode{node}
	if err := emitter.Emit(ctx, d.Store,
		emitter.WithRoots(roots),
		emitter.WithDeltas(diff.Diff(roots, prev)),
		emitter.WithCursorHash(diff.CursorHash(roots)),
		emitter.WithMessage(msg),
	); err != nil {
		return err
	}

	d.touchSnapshot()
	return nil
}

// Resolve a token budget: explicit call arg > configured default > built-in.
func resolveBudget(arg int, cfg *int, builtin int) int {
	if arg > 0 {
		return arg
	}
	if cfg != nil && *cfg > 0 {
		return *cfg
	}

	return builtin
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
