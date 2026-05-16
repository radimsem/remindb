package bench

import (
	"slices"
	"testing"
)

func TestStripRemindbEnv(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"REMINDB_SOURCE=/home/u/.claude/projects",
		"HOME=/home/u",
		"REMINDB_TRANSPORT=http",
		"REMINDB_DB=/elsewhere/m.db",
		"REMINDBX=kept", // no underscore — must NOT be stripped
		"REMINDB_=edge",
	}
	want := []string{"PATH=/usr/bin", "HOME=/home/u", "REMINDBX=kept"}

	got := stripRemindbEnv(in)
	if !slices.Equal(got, want) {
		t.Fatalf("stripRemindbEnv leaked or over-stripped\n got: %v\nwant: %v", got, want)
	}
}
