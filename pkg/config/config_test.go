package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, workspace, content string) {
	t.Helper()

	dir := filepath.Join(workspace, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_MissingDir(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(cfg, Config{}) {
		t.Errorf("expected zero-value Config, got %+v", cfg)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, DirName), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Errorf("expected zero-value Config, got %+v", cfg)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, "")

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Errorf("expected zero-value Config, got %+v", cfg)
	}
}

func TestLoad_EmptyObject(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, "{}")

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Errorf("expected zero-value Config, got %+v", cfg)
	}
}

func TestLoad_EmptyRedactionBlock(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"redaction": {}}`)

	if _, err := Load(ws); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, "{not json")

	_, err := Load(ws)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("expected 'failed to parse' prefix, got: %v", err)
	}
}

func TestLoad_UnknownTopLevelKey(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"redact": {}}`)

	_, err := Load(ws)
	if err == nil {
		t.Fatal("expected unknown-key error")
	}

	if !strings.Contains(err.Error(), `unknown key "redact"`) {
		t.Errorf(`expected 'unknown key "redact"' in error, got: %v`, err)
	}
	if strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("unknown-key error should not carry 'failed to parse' prefix, got: %v", err)
	}
}

func TestLoad_UnknownNestedKey(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"redaction": {"unknown": true}}`)

	_, err := Load(ws)
	if err == nil {
		t.Fatal("expected unknown-key error")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("expected 'unknown key' in error, got: %v", err)
	}
}

func TestLoad_TemperatureBlock_AllFields(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{
		"temperature": {
			"decay_rate": 0.03,
			"access_boost": 0.2,
			"cold_threshold": 0.08,
			"notify_threshold": 0.07,
			"summarize_rebound": 0.6,
			"tick_interval": "10m",
			"cold_notify_ttl": "2h",
			"cold_notify_limit": 100
		}
	}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc := cfg.Temperature
	if tc.DecayRate == nil || *tc.DecayRate != 0.03 {
		t.Errorf("DecayRate = %v, want 0.03", tc.DecayRate)
	}
	if tc.SummarizeRebound == nil || *tc.SummarizeRebound != 0.6 {
		t.Errorf("SummarizeRebound = %v, want 0.6", tc.SummarizeRebound)
	}
	if tc.TickInterval == nil || time.Duration(*tc.TickInterval) != 10*time.Minute {
		t.Errorf("TickInterval = %v, want 10m", tc.TickInterval)
	}
	if tc.ColdNotifyTTL == nil || time.Duration(*tc.ColdNotifyTTL) != 2*time.Hour {
		t.Errorf("ColdNotifyTTL = %v, want 2h", tc.ColdNotifyTTL)
	}
	if tc.ColdNotifyLimit == nil || *tc.ColdNotifyLimit != 100 {
		t.Errorf("ColdNotifyLimit = %v, want 100", tc.ColdNotifyLimit)
	}
}

func TestLoad_TemperatureBlock_PartialLeavesRestNil(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"temperature": {"decay_rate": 0.01}}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc := cfg.Temperature
	if tc.DecayRate == nil || *tc.DecayRate != 0.01 {
		t.Errorf("DecayRate = %v, want 0.01", tc.DecayRate)
	}

	if tc.AccessBoost != nil || tc.TickInterval != nil || tc.ColdNotifyLimit != nil {
		t.Error("unset fields should remain nil")
	}
}

func TestLoad_TemperatureBlock_UnknownNestedKeyRejected(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"temperature": {"decay_rat": 0.01}}`)

	_, err := Load(ws)
	if err == nil {
		t.Fatal("expected unknown-key error for typo'd field")
	}

	if !strings.Contains(err.Error(), `unknown key "decay_rat"`) {
		t.Errorf(`expected 'unknown key "decay_rat"', got: %v`, err)
	}
}

func TestLoad_TemperatureBlock_BadDuration(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"temperature": {"tick_interval": "notaduration"}}`)

	_, err := Load(ws)
	if err == nil {
		t.Fatal("expected parse error for malformed duration")
	}

	if !strings.Contains(err.Error(), "invalid duration") {
		t.Errorf("expected 'invalid duration' in error, got: %v", err)
	}
}

func TestLoad_RedactionBlock_Fields(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{
		"redaction": {
			"disable_builtin_kinds": ["env_secret_assignment", "jwt"],
			"custom": [{"kind": "internal_token", "pattern": "INT-[0-9a-f]{32}"}]
		}
	}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rc := cfg.Redaction
	if !reflect.DeepEqual(rc.DisableBuiltinKinds, []string{"env_secret_assignment", "jwt"}) {
		t.Errorf("DisableBuiltinKinds = %v, want [env_secret_assignment jwt]", rc.DisableBuiltinKinds)
	}

	want := []RedactionPattern{{Kind: "internal_token", Pattern: "INT-[0-9a-f]{32}"}}
	if !reflect.DeepEqual(rc.Custom, want) {
		t.Errorf("Custom = %v, want %v", rc.Custom, want)
	}
}

func TestLoad_RedactionBlock_AbsentLeavesZero(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"temperature": {"decay_rate": 0.01}}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rc := cfg.Redaction
	if rc.DisableBuiltinKinds != nil || rc.Custom != nil {
		t.Errorf("absent redaction block should leave zero value, got %+v", rc)
	}
}

func TestLoad_RedactionBlock_UnknownNestedKeyRejected(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"redaction": {"disabel_builtin_kinds": ["jwt"]}}`)

	_, err := Load(ws)
	if err == nil {
		t.Fatal("expected unknown-key error for typo'd field")
	}

	if !strings.Contains(err.Error(), `unknown key "disabel_builtin_kinds"`) {
		t.Errorf(`expected 'unknown key "disabel_builtin_kinds"', got: %v`, err)
	}
}

func TestLoad_BudgetsBlock(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"budgets": {"search": 1500, "fetch": 800, "fetch_batch": 4000, "related": 1200}}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bc := cfg.Budgets
	if bc.Search == nil || *bc.Search != 1500 {
		t.Errorf("search = %v, want 1500", bc.Search)
	}
	if bc.Fetch == nil || *bc.Fetch != 800 {
		t.Errorf("fetch = %v, want 800", bc.Fetch)
	}
	if bc.FetchBatch == nil || *bc.FetchBatch != 4000 {
		t.Errorf("fetch_batch = %v, want 4000", bc.FetchBatch)
	}
	if bc.Related == nil || *bc.Related != 1200 {
		t.Errorf("related = %v, want 1200", bc.Related)
	}
}

func TestLoad_BudgetsBlock_PartialLeavesRestNil(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"budgets": {"search": 500}}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bc := cfg.Budgets
	if bc.Search == nil || *bc.Search != 500 {
		t.Errorf("search = %v, want 500", bc.Search)
	}
	if bc.Fetch != nil || bc.FetchBatch != nil || bc.Related != nil {
		t.Error("unset fields should remain nil")
	}
}

func TestLoad_BudgetsBlock_AbsentLeavesZero(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"temperature": {"decay_rate": 0.01}}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Budgets != (BudgetsConfig{}) {
		t.Errorf("absent budgets block should leave zero value, got %+v", cfg.Budgets)
	}
}

func TestLoad_BudgetsBlock_UnknownNestedKeyRejected(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"budgets": {"serach": 500}}`)

	_, err := Load(ws)
	if err == nil {
		t.Fatal("expected unknown-key error for typo'd field")
	}

	if !strings.Contains(err.Error(), `unknown key "serach"`) {
		t.Errorf(`expected 'unknown key "serach"', got: %v`, err)
	}
}

func TestValidate_BudgetsBlock(t *testing.T) {
	zero := 0
	neg := -100

	bad := []Config{
		{Budgets: BudgetsConfig{Search: &zero}},
		{Budgets: BudgetsConfig{Fetch: &neg}},
		{Budgets: BudgetsConfig{FetchBatch: &zero}},
		{Budgets: BudgetsConfig{Related: &neg}},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected validation error, got nil", i)
		}
	}
}

func TestParseByteSize(t *testing.T) {
	ok := map[string]int64{
		"1024":  1024,
		"512B":  512,
		"2KB":   2 << 10,
		"1.5KB": 1536,
		"500MB": 500 << 20,
		"2GB":   2 << 30,
		"1GiB":  1 << 30,
		" 4MB ": 4 << 20,
		"3 mb":  3 << 20,
	}

	for in, want := range ok {
		got, err := parseByteSize(in)
		if err != nil {
			t.Errorf("parseByteSize(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseByteSize(%q) = %d, want %d", in, got, want)
		}
	}

	for _, in := range []string{"", "abc", "-5MB", "MB"} {
		if _, err := parseByteSize(in); err == nil {
			t.Errorf("parseByteSize(%q): expected error, got nil", in)
		}
	}
}

func TestLoad_CompileBlock(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"compile": {"max_file_size": "2GB", "max_parallelism": 4, "wall_clock_timeout": "30s"}}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cc := cfg.Compile
	if cc.MaxFileSize == nil || int64(*cc.MaxFileSize) != 2<<30 {
		t.Errorf("max_file_size = %v, want 2GiB", cc.MaxFileSize)
	}
	if cc.MaxParallelism == nil || *cc.MaxParallelism != 4 {
		t.Errorf("max_parallelism = %v, want 4", cc.MaxParallelism)
	}
	if cc.WallClockTimeout == nil || time.Duration(*cc.WallClockTimeout) != 30*time.Second {
		t.Errorf("wall_clock_timeout = %v, want 30s", cc.WallClockTimeout)
	}
}

func TestLoad_CompileBlock_AbsentLeavesZero(t *testing.T) {
	ws := t.TempDir()
	writeConfig(t, ws, `{"temperature": {"decay_rate": 0.01}}`)

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Compile != (CompileConfig{}) {
		t.Errorf("absent compile block should leave zero value, got %+v", cfg.Compile)
	}
}

func TestValidate_CompileBlock(t *testing.T) {
	zero := ByteSize(0)
	zeroPar := 0
	negTimeout := Duration(-time.Second)

	bad := []Config{
		{Compile: CompileConfig{MaxFileSize: &zero}},
		{Compile: CompileConfig{MaxParallelism: &zeroPar}},
		{Compile: CompileConfig{WallClockTimeout: &negTimeout}},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected validation error, got nil", i)
		}
	}

	if err := (Config{}).Validate(); err != nil {
		t.Errorf("zero-value Config should validate, got %v", err)
	}
}
