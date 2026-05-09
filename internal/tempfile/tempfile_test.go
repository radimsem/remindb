package tempfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	dir := t.TempDir()

	data := []byte(`{
		"*": 0.3,
		"README.md": 0.9,
		"architecture.md": 0.85,
		"notes": 0.2,
		"src": {
			"*": 0.6,
			"api": {
				"*": 0.7,
				"routes.yaml": 0.95,
				"deprecated.json": 0.1
			},
			"internal": 0.4
		}
	}`)
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		path string
		want float64
		ok   bool
	}{
		{"README.md", 0.9, true},
		{"architecture.md", 0.85, true},
		{"changelog.md", 0.3, true},
		{"notes/meeting.md", 0.2, true},
		{"notes/deep/nested.md", 0.2, true},
		{"src/utils.go", 0.6, true},
		{"src/api/routes.yaml", 0.95, true},
		{"src/api/deprecated.json", 0.1, true},
		{"src/api/health.json", 0.7, true},
		{"src/internal/core.go", 0.4, true},
		{"other/random.md", 0.3, true},
	}

	for _, tt := range tests {
		got, ok := r.Resolve(tt.path)
		if ok != tt.ok {
			t.Errorf("Resolve(%q): ok = %v, want %v", tt.path, ok, tt.ok)
			continue
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %g, want %g", tt.path, got, tt.want)
		}
	}
}

func TestResolve_NoGlob(t *testing.T) {
	dir := t.TempDir()

	data := []byte(`{"README.md": 0.9}`)
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got, ok := r.Resolve("README.md")
	if !ok || got != 0.9 {
		t.Errorf("Resolve(README.md) = %g, %v; want 0.9, true", got, ok)
	}

	_, ok = r.Resolve("other.md")
	if ok {
		t.Error("Resolve(other.md) should return false with no glob")
	}
}

func TestResolve_NilResolver(t *testing.T) {
	var r *Resolver
	_, ok := r.Resolve("anything.md")
	if ok {
		t.Error("nil resolver should return false")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	r, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r != nil {
		t.Error("expected nil resolver for missing file")
	}
}

func TestLoad_InvalidJson(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"x": 1.5}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for value > 1")
	}

	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, FileName), []byte(`{"x": -0.1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(dir2)
	if err == nil {
		t.Fatal("expected error for value < 0")
	}
}

func TestLoad_InvalidType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"x": "hot"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for string value")
	}
}

func TestResolve_EmptyObject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, ok := r.Resolve("anything.md")
	if ok {
		t.Error("empty object should not resolve anything")
	}
}

func TestResolve_FlatSlashKey(t *testing.T) {
	dir := t.TempDir()

	data := []byte(`{
		"src/api/routes.yaml": 0.95,
		"src/api/deprecated.json": 0.1,
		"docs/architecture.md": 0.85
	}`)
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		path string
		want float64
		ok   bool
	}{
		{"src/api/routes.yaml", 0.95, true},
		{"src/api/deprecated.json", 0.1, true},
		{"docs/architecture.md", 0.85, true},
		{"src/api/missing.yaml", 0, false},
		{"src/other.go", 0, false},
	}

	for _, tt := range tests {
		got, ok := r.Resolve(tt.path)
		if ok != tt.ok {
			t.Errorf("Resolve(%q): ok = %v, want %v", tt.path, ok, tt.ok)
			continue
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %g, want %g", tt.path, got, tt.want)
		}
	}
}

func TestResolve_SlashKeyWithGlob(t *testing.T) {
	dir := t.TempDir()

	data := []byte(`{
		"src/*": 0.6,
		"src/api/*": 0.7,
		"src/api/routes.yaml": 0.95
	}`)
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		path string
		want float64
		ok   bool
	}{
		{"src/utils.go", 0.6, true},
		{"src/api/routes.yaml", 0.95, true},
		{"src/api/health.json", 0.7, true},
	}

	for _, tt := range tests {
		got, ok := r.Resolve(tt.path)
		if ok != tt.ok {
			t.Errorf("Resolve(%q): ok = %v, want %v", tt.path, ok, tt.ok)
			continue
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %g, want %g", tt.path, got, tt.want)
		}
	}
}

func TestResolve_MixedSlashAndNested(t *testing.T) {
	dir := t.TempDir()

	data := []byte(`{
		"src/api/routes.yaml": 0.95,
		"src": {
			"api": {
				"deprecated.json": 0.1
			},
			"internal": 0.4
		}
	}`)
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		path string
		want float64
		ok   bool
	}{
		{"src/api/routes.yaml", 0.95, true},
		{"src/api/deprecated.json", 0.1, true},
		{"src/internal/core.go", 0.4, true},
	}

	for _, tt := range tests {
		got, ok := r.Resolve(tt.path)
		if ok != tt.ok {
			t.Errorf("Resolve(%q): ok = %v, want %v", tt.path, ok, tt.ok)
			continue
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %g, want %g", tt.path, got, tt.want)
		}
	}
}

func TestLoad_ConflictingTemps(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "slash and nested set same leaf",
			json: `{"src/api/routes.yaml": 0.95, "src": {"api": {"routes.yaml": 0.5}}}`,
		},
		{
			name: "two slash-keys disagree",
			json: `{"src/x": 0.1, "src": {"x": 0.9}}`,
		},
		{
			name: "leaf shadows deeper override",
			json: `{"src/api": 0.7, "src/api/x.go": 0.1}`,
		},
		{
			name: "deeper override under leaf, reversed",
			json: `{"src/api/x.go": 0.1, "src/api": 0.7}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, FileName), []byte(tt.json), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("expected error for conflicting temperatures")
			}
		})
	}
}

func TestResolve_NormalizedKeys(t *testing.T) {
	dir := t.TempDir()

	data := []byte(`{
		"./README.md": 0.9,
		"src/api/": 0.7,
		"docs/": 0.4
	}`)
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		path string
		want float64
		ok   bool
	}{
		{"README.md", 0.9, true},
		{"src/api/routes.yaml", 0.7, true},
		{"docs/architecture.md", 0.4, true},
	}

	for _, tt := range tests {
		got, ok := r.Resolve(tt.path)
		if ok != tt.ok {
			t.Errorf("Resolve(%q): ok = %v, want %v", tt.path, ok, tt.ok)
			continue
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %g, want %g", tt.path, got, tt.want)
		}
	}
}

func TestLoad_InvalidSegment(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"empty segment", `{"src//routes.yaml": 0.5}`},
		{"leading slash", `{"/README.md": 0.5}`},
		{"empty after normalization", `{"./": 0.5}`},
		{"mid-path dot", `{"src/./README.md": 0.5}`},
		{"dotdot segment", `{"../x": 0.5}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, FileName), []byte(tt.json), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("expected error for invalid segment")
			}
		})
	}
}
