package query

import "github.com/radimsem/remindb/pkg/store"

const (
	opAdd = "add"
	opMod = "mod"
	opRem = "rem"
)

// Collapse per-snapshot diff rows in a range into one record per changed node.
func consolidateDiffs(raw []*store.DiffRecord) []*store.DiffRecord {
	if len(raw) == 0 {
		return nil
	}

	type bracket struct {
		first, last *store.DiffRecord
	}

	byNode := make(map[string]*bracket, len(raw))
	order := make([]string, 0, len(raw))

	for _, d := range raw {
		b, ok := byNode[d.NodeID]
		if !ok {
			byNode[d.NodeID] = &bracket{first: d, last: d}
			order = append(order, d.NodeID)

			continue
		}

		b.last = d
	}

	out := make([]*store.DiffRecord, 0, len(byNode))
	for _, id := range order {
		b := byNode[id]
		if rec, ok := consolidateBracket(b.first, b.last); ok {
			out = append(out, rec)
		}
	}

	return out
}

func consolidateBracket(first, last *store.DiffRecord) (*store.DiffRecord, bool) {
	existedAtFrom := first.Op != opAdd
	existsAtTo := last.Op != opRem

	switch {
	case !existedAtFrom && !existsAtTo:
		return nil, false
	case !existedAtFrom && existsAtTo:
		return &store.DiffRecord{
			SnapshotID: last.SnapshotID,
			NodeID:     first.NodeID,
			Op:         opAdd,
			NewHash:    last.NewHash,
			NewContent: last.NewContent,
		}, true
	case existedAtFrom && !existsAtTo:
		return &store.DiffRecord{
			SnapshotID: last.SnapshotID,
			NodeID:     first.NodeID,
			Op:         opRem,
			OldHash:    first.OldHash,
			OldContent: first.OldContent,
		}, true
	}

	if first.OldContent == last.NewContent {
		return nil, false
	}
	return &store.DiffRecord{
		SnapshotID: last.SnapshotID,
		NodeID:     first.NodeID,
		Op:         opMod,
		OldHash:    first.OldHash,
		NewHash:    last.NewHash,
		OldContent: first.OldContent,
		NewContent: last.NewContent,
	}, true
}
