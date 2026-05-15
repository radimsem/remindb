package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func TestConsolidateDiffs(t *testing.T) {
	tests := []struct {
		name string
		raw  []*store.DiffRecord
		want []*store.DiffRecord
	}{
		{
			name: "empty",
			raw:  nil,
			want: nil,
		},
		{
			name: "single add passes through",
			raw: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "n1", Op: "add", NewHash: "h1", NewContent: "v1"},
			},
			want: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "n1", Op: "add", NewHash: "h1", NewContent: "v1"},
			},
		},
		{
			name: "add then mod collapses to add with final content",
			raw: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "n1", Op: "add", NewHash: "h1", NewContent: "v1"},
				{SnapshotID: 2, NodeID: "n1", Op: "mod", OldHash: "h1", NewHash: "h2", OldContent: "v1", NewContent: "v2"},
			},
			want: []*store.DiffRecord{
				{SnapshotID: 2, NodeID: "n1", Op: "add", NewHash: "h2", NewContent: "v2"},
			},
		},
		{
			name: "add then rem is dropped",
			raw: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "n1", Op: "add", NewHash: "h1", NewContent: "v1"},
				{SnapshotID: 2, NodeID: "n1", Op: "rem", OldHash: "h1", OldContent: "v1"},
			},
			want: nil,
		},
		{
			name: "mod then mod surfaces only outer endpoints",
			raw: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "n1", Op: "mod", OldHash: "h0", NewHash: "h1", OldContent: "v0", NewContent: "v1"},
				{SnapshotID: 2, NodeID: "n1", Op: "mod", OldHash: "h1", NewHash: "h2", OldContent: "v1", NewContent: "v2"},
			},
			want: []*store.DiffRecord{
				{SnapshotID: 2, NodeID: "n1", Op: "mod", OldHash: "h0", NewHash: "h2", OldContent: "v0", NewContent: "v2"},
			},
		},
		{
			name: "mod then rem collapses to rem",
			raw: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "n1", Op: "mod", OldHash: "h0", NewHash: "h1", OldContent: "v0", NewContent: "v1"},
				{SnapshotID: 2, NodeID: "n1", Op: "rem", OldHash: "h1", OldContent: "v1"},
			},
			want: []*store.DiffRecord{
				{SnapshotID: 2, NodeID: "n1", Op: "rem", OldHash: "h0", OldContent: "v0"},
			},
		},
		{
			name: "oscillates back to identical content is dropped",
			raw: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "n1", Op: "mod", OldHash: "h0", NewHash: "h1", OldContent: "v0", NewContent: "v1"},
				{SnapshotID: 2, NodeID: "n1", Op: "mod", OldHash: "h1", NewHash: "h0", OldContent: "v1", NewContent: "v0"},
			},
			want: nil,
		},
		{
			name: "preserves first-seen node order across multiple nodes",
			raw: []*store.DiffRecord{
				{SnapshotID: 1, NodeID: "first", Op: "add", NewHash: "h_f", NewContent: "f"},
				{SnapshotID: 2, NodeID: "second", Op: "add", NewHash: "h_s", NewContent: "s"},
				{SnapshotID: 3, NodeID: "first", Op: "mod", OldHash: "h_f", NewHash: "h_f2", OldContent: "f", NewContent: "f2"},
			},
			want: []*store.DiffRecord{
				{SnapshotID: 3, NodeID: "first", Op: "add", NewHash: "h_f2", NewContent: "f2"},
				{SnapshotID: 2, NodeID: "second", Op: "add", NewHash: "h_s", NewContent: "s"},
			},
		},
		{
			name: "rem alone surfaces with pre-range state",
			raw: []*store.DiffRecord{
				{SnapshotID: 5, NodeID: "n1", Op: "rem", OldHash: "h_old", OldContent: "kept value"},
			},
			want: []*store.DiffRecord{
				{SnapshotID: 5, NodeID: "n1", Op: "rem", OldHash: "h_old", OldContent: "kept value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consolidateDiffs(tt.raw)

			if !diffRecordsEqual(got, tt.want) {
				t.Errorf("consolidateDiffs() mismatch\ngot:  %s\nwant: %s", fmtRecords(got), fmtRecords(tt.want))
			}
		})
	}
}

func diffRecordsEqual(a, b []*store.DiffRecord) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if *a[i] != *b[i] {
			return false
		}
	}
	return true
}

func fmtRecords(rs []*store.DiffRecord) string {
	if len(rs) == 0 {
		return "[]"
	}

	var b strings.Builder
	b.WriteByte('[')

	for i, r := range rs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "{%s/%s old=%s new=%s}", r.NodeID, r.Op, r.OldContent, r.NewContent)
	}

	b.WriteByte(']')
	return b.String()
}
