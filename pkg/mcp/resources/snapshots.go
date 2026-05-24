package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/store"
)

const (
	SnapshotsURI           = "remindb://snapshots"
	SnapshotsLimitTemplate = "remindb://snapshots{?limit}"
	SnapshotDiffsTemplate  = "remindb://snapshots/{id}/diffs"

	// SQLite reads a negative LIMIT as "no upper bound" — the full history.
	snapshotListUnlimited = -1
)

type snapshotEntry struct {
	ID          int64  `json:"id"`
	ParentID    *int64 `json:"parent_id"`
	CreatedAt   int64  `json:"created_at"`
	Message     string `json:"message"`
	CompileRoot string `json:"compile_root"`
	IsHead      bool   `json:"is_head"`
}

type snapshotListEnvelope struct {
	Snapshots []snapshotEntry `json:"snapshots"`
}

type diffEntry struct {
	Op         string `json:"op"`
	NodeID     string `json:"node_id"`
	OldHash    string `json:"old_hash"`
	NewHash    string `json:"new_hash"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
}

type snapshotDiffsEnvelope struct {
	SnapshotID int64       `json:"snapshot_id"`
	Diffs      []diffEntry `json:"diffs"`
}

// Map store snapshots into the locked JSON envelope, marking HEAD and preserving parent links.
func newSnapshotsEnvelope(snaps []*store.Snapshot, headID int64) snapshotListEnvelope {
	out := make([]snapshotEntry, 0, len(snaps))
	for _, s := range snaps {
		e := snapshotEntry{
			ID:          s.ID,
			Message:     s.Message,
			CompileRoot: s.CompileRoot,
			CreatedAt:   s.CreatedAt,
			IsHead:      headID != 0 && s.ID == headID,
		}

		if s.ParentID.Valid {
			pid := s.ParentID.Int64
			e.ParentID = &pid
		}

		out = append(out, e)
	}

	return snapshotListEnvelope{Snapshots: out}
}

func newSnapshotDiffsEnvelope(snapshotID int64, diffs []*store.DiffRecord) snapshotDiffsEnvelope {
	out := make([]diffEntry, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, diffEntry{
			Op:         d.Op,
			NodeID:     d.NodeID,
			OldHash:    d.OldHash,
			NewHash:    d.NewHash,
			OldContent: d.OldContent,
			NewContent: d.NewContent,
		})
	}

	return snapshotDiffsEnvelope{SnapshotID: snapshotID, Diffs: out}
}

func (d *Deps) snapshotsBody(ctx context.Context, limit int) ([]byte, error) {
	snaps, err := d.Store.ListSnapshots(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list: snapshots: %w", err)
	}

	headID, err := d.Store.GetHeadSnapshotID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: head snapshot id: %w", err)
	}

	body, err := json.Marshal(newSnapshotsEnvelope(snaps, headID))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: snapshots: %w", err)
	}
	return body, nil
}

func (d *Deps) HandleSnapshots(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	body, err := d.snapshotsBody(ctx, snapshotListUnlimited)
	if err != nil {
		return nil, err
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      SnapshotsURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}

func (d *Deps) HandleSnapshotsLimited(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: snapshots uri: %w", err)
	}

	limit := snapshotListUnlimited
	if ls := u.Query().Get("limit"); ls != "" {
		limit, err = strconv.Atoi(ls)
		if err != nil || limit <= 0 {
			return nil, fmt.Errorf("invalid limit %q", ls)
		}
	}

	body, err := d.snapshotsBody(ctx, limit)
	if err != nil {
		return nil, err
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}

func (d *Deps) HandleSnapshotDiffs(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: snapshot diffs uri: %w", err)
	}

	seg := strings.TrimPrefix(u.Path, "/")
	seg = strings.TrimSuffix(seg, "/diffs")
	if seg == "" {
		return nil, fmt.Errorf("snapshot diffs uri missing snapshot id")
	}

	id, err := strconv.ParseInt(seg, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid snapshot id %q", seg)
	}

	diffs, err := d.Store.GetDiffsBySnapshot(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get: snapshot %d diffs: %w", id, err)
	}

	body, err := json.Marshal(newSnapshotDiffsEnvelope(id, diffs))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: snapshot diffs: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
