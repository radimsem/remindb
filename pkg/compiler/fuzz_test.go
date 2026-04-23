package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
)

func FuzzCompile(f *testing.F) {
	f.Add("doc.md", []byte("# Hello\n\nContent.\n"))
	f.Add("data.yaml", []byte("key: value\nnested:\n  a: 1\n"))
	f.Add("data.json", []byte(`{"name":"test","items":[1,2,3]}`))
	f.Add("file.toon", []byte("col1\tcol2\nval1\tval2\n"))
	f.Add("skip.txt", []byte("unsupported extension"))
	f.Add("empty.md", []byte(""))
	f.Add("bad.json", []byte(`{"unclosed":`))
	f.Add("bom.md", []byte{0xef, 0xbb, 0xbf, '#', ' ', 'x', '\n'})
	f.Add("front.md", []byte("---\nkey: val\n---\n# body\n"))
	f.Add("deep.json", []byte(`{"a":{"b":{"c":{"d":{"e":"deep"}}}}}`))
	f.Add("invalid.md", []byte{0xe3, 0x80})
	f.Add("mixed.yaml", []byte("- a\n- b\n- nested:\n    key: val"))

	f.Fuzz(func(t *testing.T, name string, data []byte) {
		// Guard against path escapes; the fuzz input is a file name, not a path.
		if name == "" || name == "." || name == ".." ||
			strings.ContainsAny(name, "/\\") ||
			strings.HasPrefix(name, ".") {
			t.Skip()
		}

		st := testutil.OpenTestDB(t)
		ctx := context.Background()
		dir := t.TempDir()

		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Skip()
		}

		snapsBefore, _ := st.ListSnapshots(ctx, 10)
		rootsBefore, _ := st.GetRootNodes(ctx)

		// Must never panic regardless of input.
		_, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("fuzz"))

		// Atomicity: on error, the store must be unchanged.
		if err != nil {
			snapsAfter, _ := st.ListSnapshots(ctx, 10)
			if len(snapsAfter) != len(snapsBefore) {
				t.Errorf("Compile errored but snapshots changed: before=%d after=%d err=%v",
					len(snapsBefore), len(snapsAfter), err)
			}

			rootsAfter, _ := st.GetRootNodes(ctx)
			if len(rootsAfter) != len(rootsBefore) {
				t.Errorf("Compile errored but root nodes changed: before=%d after=%d err=%v",
					len(rootsBefore), len(rootsAfter), err)
			}
		}
	})
}

func FuzzCompileDir(f *testing.F) {
	f.Add([]byte("# A\n"), []byte("key: val\n"), []byte(`{"k":1}`))
	f.Add([]byte(""), []byte(""), []byte(""))
	f.Add([]byte("## only h2\n"), []byte("- a\n- b"), []byte(`[1,2,3]`))
	f.Add([]byte{0xff, 0xfe}, []byte("bad: :yaml"), []byte(`{"bad"`))

	f.Fuzz(func(t *testing.T, md, yaml, json []byte) {
		st := testutil.OpenTestDB(t)
		ctx := context.Background()
		dir := t.TempDir()

		if err := os.WriteFile(filepath.Join(dir, "a.md"), md, 0o644); err != nil {
			t.Skip()
		}
		if err := os.WriteFile(filepath.Join(dir, "b.yaml"), yaml, 0o644); err != nil {
			t.Skip()
		}
		if err := os.WriteFile(filepath.Join(dir, "c.json"), json, 0o644); err != nil {
			t.Skip()
		}
		_ = os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("ignored"), 0o644)

		snapsBefore, _ := st.ListSnapshots(ctx, 10)

		_, err := CompileDir(ctx, st, dir, "fuzz-dir")

		if err != nil {
			snapsAfter, _ := st.ListSnapshots(ctx, 10)
			if len(snapsAfter) != len(snapsBefore) {
				t.Errorf("CompileDir errored but snapshots changed: before=%d after=%d err=%v",
					len(snapsBefore), len(snapsAfter), err)
			}
		}
	})
}
