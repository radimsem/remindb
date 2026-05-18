package doctor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func newCleanStore(t *testing.T) (*store.Store, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "doctor.db")

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st, path
}

func TestRunCleanDB(t *testing.T) {
	st, _ := newCleanStore(t)

	report := Run(context.Background(), st)

	want := map[string]string{
		"fts5_sync":          "pass",
		"orphan_parent_id":   "pass",
		"head_cursor":        "pass",
		"dangling_diffs":     "pass",
		"schema_version":     "pass",
		"stale_compile_root": "pass",
	}

	if len(report.Checks) != len(want) {
		t.Fatalf("checks count: got %d, want %d", len(report.Checks), len(want))
	}

	for _, c := range report.Checks {
		w, ok := want[c.Name]
		if !ok {
			t.Errorf("unexpected check %q", c.Name)
			continue
		}

		if c.Status != w {
			t.Errorf("%s: got %s, want %s (%s)", c.Name, c.Status, w, c.Detail)
		}
	}

	if report.HasFailures() {
		t.Errorf("clean DB reports failures")
	}
}

func TestHealFixesBrokenFTS(t *testing.T) {
	st, path := newCleanStore(t)

	insertSampleNodes(t, path)

	if err := breakFTSSync(path); err != nil {
		t.Fatalf("breakFTSSync: %v", err)
	}

	pre := Run(context.Background(), st)
	if !containsFailure(pre, "fts5_sync") {
		t.Fatalf("expected fts5_sync to fail before heal")
	}

	post := Heal(context.Background(), st)

	for _, c := range post.Checks {
		if c.Name != "fts5_sync" {
			continue
		}
		if c.Status != "pass" {
			t.Errorf("fts5_sync after heal: got %s (%s)", c.Status, c.Detail)
		}
		if !c.FixApplied {
			t.Errorf("fts5_sync: expected FixApplied=true")
		}
	}

	if post.HasFailures() {
		t.Errorf("post-heal has failures: %+v", post.Checks)
	}
}

func TestHealIsIdempotent(t *testing.T) {
	st, _ := newCleanStore(t)

	first := Heal(context.Background(), st)
	second := Heal(context.Background(), st)

	if first.HasFailures() || second.HasFailures() {
		t.Errorf("idempotent heal should not produce failures")
	}

	for _, c := range second.Checks {
		if c.FixApplied {
			t.Errorf("second heal applied a fix on a healthy DB: %+v", c)
		}
	}
}

func TestReportJSONShape(t *testing.T) {
	st, _ := newCleanStore(t)

	report := Run(context.Background(), st)

	var buf bytes.Buffer
	if err := report.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded struct {
		Status string           `json:"status"`
		Checks []map[string]any `json:"checks"`
	}

	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Status != report.Status().String() {
		t.Errorf("status header: got %q, want %q", decoded.Status, report.Status().String())
	}
	if len(decoded.Checks) != len(report.Checks) {
		t.Fatalf("round-trip count: got %d, want %d", len(decoded.Checks), len(report.Checks))
	}

	for _, entry := range decoded.Checks {
		for _, k := range []string{"name", "status", "detail"} {
			if _, ok := entry[k]; !ok {
				t.Errorf("missing key %q in %+v", k, entry)
			}
		}
	}
}

func TestReportTextShape(t *testing.T) {
	st, _ := newCleanStore(t)

	report := Run(context.Background(), st)

	var buf bytes.Buffer
	if err := report.WriteText(&buf, false); err != nil {
		t.Fatalf("WriteText: %v", err)
	}

	out := buf.String()
	if len(out) == 0 {
		t.Fatalf("empty text report")
	}

	for _, c := range report.Checks {
		if !bytes.Contains(buf.Bytes(), []byte(c.Name)) {
			t.Errorf("text report missing check name %q", c.Name)
		}
	}
}

func TestDiffSorted(t *testing.T) {
	cases := []struct {
		want, have []string
		expect     []string
	}{
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}, nil},
		{[]string{"a", "b", "c"}, []string{"a", "c"}, []string{"b"}},
		{[]string{"a", "b", "c"}, []string{}, []string{"a", "b", "c"}},
	}

	for i, tc := range cases {
		got := diffSorted(tc.want, tc.have)

		if !equalStringSlice(got, tc.expect) {
			t.Errorf("case %d: got %v, want %v", i, got, tc.expect)
		}
	}
}

func TestStatusWorstWins(t *testing.T) {
	mk := func(statuses ...Status) Report {
		checks := make([]CheckReport, 0, len(statuses))
		for _, s := range statuses {
			checks = append(checks, CheckReport{Status: s.String()})
		}
		return Report{Checks: checks}
	}

	cases := []struct {
		name string
		in   Report
		want Status
	}{
		{"empty", mk(), Pass},
		{"all pass", mk(Pass, Pass), Pass},
		{"has warn", mk(Pass, Warn, Pass), Warn},
		{"has fail", mk(Pass, Fail, Pass), Fail},
		{"fail beats warn", mk(Warn, Fail, Warn), Fail},
	}

	for _, tc := range cases {
		if got := tc.in.Status(); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestWriteTextHeader(t *testing.T) {
	cases := []struct {
		name   string
		report Report
		header string
	}{
		{"healthy", Report{Checks: []CheckReport{{Name: "a", Status: "pass"}}}, "✓ Database is healthy"},
		{"warnings", Report{Checks: []CheckReport{{Name: "a", Status: "warn"}}}, "⚠ Database has warnings"},
		{"unhealthy", Report{Checks: []CheckReport{{Name: "a", Status: "fail"}}}, "✗ Database is unhealthy"},
	}

	for _, tc := range cases {
		var buf bytes.Buffer
		if err := tc.report.WriteText(&buf, false); err != nil {
			t.Fatalf("%s: WriteText: %v", tc.name, err)
		}

		lines := strings.SplitN(buf.String(), "\n", 3)
		if len(lines) < 3 {
			t.Fatalf("%s: want header + blank + checklist, got %q", tc.name, buf.String())
		}

		if lines[0] != tc.header {
			t.Errorf("%s: header = %q, want %q", tc.name, lines[0], tc.header)
		}
		if lines[1] != "" {
			t.Errorf("%s: want blank line after header, got %q", tc.name, lines[1])
		}
		if !strings.Contains(lines[2], "a") {
			t.Errorf("%s: checklist missing after header, got %q", tc.name, lines[2])
		}
	}
}

func TestWriteTextHeaderHealsToHealthy(t *testing.T) {
	st, path := newCleanStore(t)

	insertSampleNodes(t, path)
	if err := breakFTSSync(path); err != nil {
		t.Fatalf("breakFTSSync: %v", err)
	}

	post := Heal(context.Background(), st)
	if post.HasFailures() {
		t.Fatalf("post-heal still has failures: %+v", post.Checks)
	}

	var buf bytes.Buffer
	if err := post.WriteText(&buf, false); err != nil {
		t.Fatalf("WriteText: %v", err)
	}

	header := strings.SplitN(buf.String(), "\n", 2)[0]
	if header != "✓ Database is healthy" {
		t.Errorf("post-fix header = %q, want %q", header, "✓ Database is healthy")
	}
}

func containsFailure(r Report, name string) bool {
	for _, c := range r.Checks {
		if c.Name == name && c.Status == "fail" {
			return true
		}
	}
	return false
}

func insertSampleNodes(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec(`
		INSERT INTO nodes (id, parent_id, source_file, node_type, depth, label, content, format, token_count, content_hash, temperature)
		VALUES
		('n0000000001', NULL, '/x', 'p', 0, 'l1', '', 'plain', 0, 'h1', 0.5),
		('n0000000002', NULL, '/x', 'p', 0, 'l2', '', 'plain', 0, 'h2', 0.5)`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func breakFTSSync(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec(`INSERT INTO nodes_fts(nodes_fts) VALUES('delete-all')`)
	return err
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
