package resources

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/doctor"
	"github.com/radimsem/remindb/pkg/store"
)

// readDoctor invokes the resource handler and returns its decoded JSON body.
func readDoctor(t *testing.T, d *Deps) map[string]any {
	t.Helper()

	res, err := d.HandleDoctor(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleDoctor: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("contents: got %d, want 1", len(res.Contents))
	}
	if res.Contents[0].URI != DoctorURI || res.Contents[0].MIMEType != mimeJSON {
		t.Fatalf("envelope: got %q/%q", res.Contents[0].URI, res.Contents[0].MIMEType)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &got); err != nil {
		t.Fatalf("unmarshal resource body: %v", err)
	}
	return got
}

func assertCLIParity(t *testing.T, st *store.Store, got map[string]any, wantStatus string) {
	t.Helper()

	if got["status"] != wantStatus {
		t.Errorf("status header: got %v, want %q", got["status"], wantStatus)
	}
	if _, ok := got["checks"]; !ok {
		t.Fatalf("missing top-level \"checks\"")
	}

	var cli bytes.Buffer
	if err := doctor.Run(context.Background(), st).WriteJSON(&cli); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var want map[string]any
	if err := json.Unmarshal(cli.Bytes(), &want); err != nil {
		t.Fatalf("unmarshal CLI body: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("resource diverges from `doctor --json`:\n resource=%v\n cli     =%v", got, want)
	}
}

func TestHandleDoctor_PassParity(t *testing.T) {
	st := testutil.OpenTestDB(t)
	d := &Deps{Store: st}

	got := readDoctor(t, d)
	assertCLIParity(t, st, got, "pass")
}

func TestHandleDoctor_WarnParity(t *testing.T) {
	st, path := testutil.OpenTestDBFile(t)

	// A snapshot whose compile_root no longer exists trips stale_compile_root → warn.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	defer func() { _ = db.Close() }()
	gone := filepath.Join(filepath.Dir(path), "vanished")
	if _, err := db.Exec(
		`INSERT INTO snapshots (cursor_hash, parent_id, message, compile_root) VALUES ('h', NULL, 'm', ?)`,
		gone,
	); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	d := &Deps{Store: st}
	got := readDoctor(t, d)
	assertCLIParity(t, st, got, "warn")
}
