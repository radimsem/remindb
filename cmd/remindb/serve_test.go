package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/redaction"
	"github.com/radimsem/remindb/pkg/config"
	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/spf13/cobra"
)

func ptr[T any](v T) *T { return &v }

func newServeTestCmd(t *testing.T) *cobra.Command {
	t.Helper()

	c := &cobra.Command{Use: "serve"}
	c.Flags().StringVar(&transport, "transport", remindb.TransportStdio, "")
	c.Flags().StringVar(&listen, "listen", remindb.DefaultListenAddr, "")
	c.Flags().BoolVar(&insecurePublic, "insecure-public", false, "")

	return c
}

func TestResolveServerConfig_FlagBeatsConfig(t *testing.T) {
	c := newServeTestCmd(t)
	if err := c.Flags().Set("transport", "http"); err != nil {
		t.Fatal(err)
	}

	if err := resolveServerConfig(c, config.ServerConfig{Transport: ptr("stdio")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport != "http" {
		t.Errorf("transport = %q, want http (flag wins)", transport)
	}
}

func TestResolveServerConfig_ConfigBeatsEnv(t *testing.T) {
	c := newServeTestCmd(t)
	t.Setenv("REMINDB_TRANSPORT", "stdio")

	if err := resolveServerConfig(c, config.ServerConfig{Transport: ptr("http")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport != "http" {
		t.Errorf("transport = %q, want http (config beats env)", transport)
	}
}

func TestResolveServerConfig_EnvBeatsDefault(t *testing.T) {
	c := newServeTestCmd(t)
	t.Setenv("REMINDB_TRANSPORT", "http")

	if err := resolveServerConfig(c, config.ServerConfig{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport != "http" {
		t.Errorf("transport = %q, want http (env beats default)", transport)
	}
}

func TestResolveServerConfig_DefaultWhenUnset(t *testing.T) {
	c := newServeTestCmd(t)

	if err := resolveServerConfig(c, config.ServerConfig{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport != remindb.TransportStdio {
		t.Errorf("transport = %q, want stdio (default)", transport)
	}
	if listen != remindb.DefaultListenAddr {
		t.Errorf("listen = %q, want %q (default)", listen, remindb.DefaultListenAddr)
	}
}

func TestResolveServerConfig_ConfigListenRequiresHTTP(t *testing.T) {
	c := newServeTestCmd(t)

	err := resolveServerConfig(c, config.ServerConfig{Listen: ptr("0.0.0.0:9000")})
	if err == nil {
		t.Fatal("expected error: listen with stdio transport")
	}
	if !strings.Contains(err.Error(), "--transport=http") {
		t.Errorf("error should mention the http requirement, got: %v", err)
	}
}

func TestResolveServerConfig_InsecurePublicTrueRequiresHTTP(t *testing.T) {
	c := newServeTestCmd(t)
	if err := c.Flags().Set("insecure-public", "true"); err != nil {
		t.Fatal(err)
	}

	err := resolveServerConfig(c, config.ServerConfig{})
	if err == nil {
		t.Fatal("expected error: insecure-public with stdio transport")
	}

	if !strings.Contains(err.Error(), "--transport=http") {
		t.Errorf("error should mention the http requirement, got: %v", err)
	}
}

func TestResolveServerConfig_InsecurePublicFalseAllowsStdio(t *testing.T) {
	c := newServeTestCmd(t)
	if err := c.Flags().Set("insecure-public", "false"); err != nil {
		t.Fatal(err)
	}

	if err := resolveServerConfig(c, config.ServerConfig{}); err != nil {
		t.Errorf("explicit --insecure-public=false must not break stdio, got: %v", err)
	}
}

func TestResolveServerConfig_UnsupportedTransport(t *testing.T) {
	c := newServeTestCmd(t)
	t.Setenv("REMINDB_TRANSPORT", "grpc")

	err := resolveServerConfig(c, config.ServerConfig{})
	if err == nil {
		t.Fatal("expected error: unsupported transport")
	}
	if !strings.Contains(err.Error(), "unsupported transport") {
		t.Errorf("error should name the unsupported transport, got: %v", err)
	}
}

func TestNewServeLogger_ConfigLevel(t *testing.T) {
	lg, file, _, err := newServeLogger(false, config.LoggingConfig{Level: ptr("warn")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file != nil {
		t.Error("no output_path set, file should be nil")
	}

	ctx := context.Background()
	if lg.Enabled(ctx, slog.LevelInfo) {
		t.Error("info should be below the configured warn level")
	}
	if !lg.Enabled(ctx, slog.LevelWarn) {
		t.Error("warn should be enabled at the configured level")
	}
}

func TestNewServeLogger_VerboseBeatsConfig(t *testing.T) {
	lg, _, _, err := newServeLogger(true, config.LoggingConfig{Level: ptr("error")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !lg.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("--verbose must force debug even when config says error")
	}
}

func TestNewServeLogger_JsonFileOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.log")

	lg, file, _, err := newServeLogger(false, config.LoggingConfig{Format: ptr("json"), OutputPath: ptr(path)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file == nil {
		t.Fatal("output_path set, file handle should be returned for cleanup")
	}
	defer func() { _ = file.Close() }()

	lg.Info("hello", "k", "v")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"msg":"hello"`) {
		t.Errorf("expected JSON output to file, got: %q", data)
	}
}

func TestNewServeLogger_OutputOpenFailsLoud(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "r.log")

	_, _, _, err := newServeLogger(false, config.LoggingConfig{OutputPath: ptr(bad)})
	if err == nil {
		t.Fatal("expected loud failure when output_path cannot be opened")
	}
	if !strings.Contains(err.Error(), bad) {
		t.Errorf("error should name the unopenable path, got: %v", err)
	}
}

func TestNewServeLogger_ConfiguredBufferCaptures(t *testing.T) {
	lg, _, buf, err := newServeLogger(false, config.LoggingConfig{BufferSize: ptr(2)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf == nil {
		t.Fatal("buffer should be returned for the logs resource")
	}

	lg.Info("a")
	lg.Info("b")
	lg.Info("c")

	recs := buf.Records()
	if len(recs) != 2 || recs[0].Msg != "b" || recs[1].Msg != "c" {
		t.Errorf("configured size 2 not honored: got %d records %v", len(recs), recs)
	}
	if buf.Dropped() != 1 {
		t.Errorf("dropped: got %d, want 1", buf.Dropped())
	}
}

func TestEffectiveLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
		lg      config.LoggingConfig
		want    slog.Level
	}{
		{"default is info", false, config.LoggingConfig{}, slog.LevelInfo},
		{"config level honored", false, config.LoggingConfig{Level: ptr("warn")}, slog.LevelWarn},
		{"verbose forces debug over config", true, config.LoggingConfig{Level: ptr("error")}, slog.LevelDebug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveLogLevel(tt.verbose, tt.lg); got != tt.want {
				t.Errorf("effectiveLogLevel = %v, want %v", got, tt.want)
			}
		})
	}
}

func attrMap(attrs []any) map[string]any {
	m := make(map[string]any, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		m[attrs[i].(string)] = attrs[i+1]
	}

	return m
}

func TestStartupAttrs_SurfacesEffectiveValues(t *testing.T) {
	prevSource, prevTransport := sourceDir, transport
	sourceDir, transport = "/some/source", remindb.TransportStdio
	defer func() { sourceDir, transport = prevSource, prevTransport }()

	attrs := startupAttrs(slog.LevelDebug, 90*time.Second, 45*time.Second, false, false)
	m := attrMap(attrs)

	if m["log_level"] != slog.LevelDebug {
		t.Errorf("log_level = %v, want %v", m["log_level"], slog.LevelDebug)
	}
	if m["rescan_interval"] != 45*time.Second {
		t.Errorf("rescan_interval = %v, want 45s", m["rescan_interval"])
	}
	if m["rescan_enabled"] != false {
		t.Errorf("rescan_enabled = %v, want false (disabled via config surfaces in the log)", m["rescan_enabled"])
	}
	if m["temperature_enabled"] != false {
		t.Errorf("temperature_enabled = %v, want false", m["temperature_enabled"])
	}
	if _, ok := m["verbose"]; ok {
		t.Error("raw verbose bool should be replaced by effective log_level")
	}
}

func TestStartupAttrs_OmitsRescanWithoutSource(t *testing.T) {
	prevSource, prevTransport := sourceDir, transport
	sourceDir, transport = "", remindb.TransportStdio
	defer func() { sourceDir, transport = prevSource, prevTransport }()

	m := attrMap(startupAttrs(slog.LevelInfo, 90*time.Second, 30*time.Second, true, true))

	if _, ok := m["rescan_interval"]; ok {
		t.Error("rescan_interval should be omitted when no --source is set")
	}
	if _, ok := m["rescan_enabled"]; ok {
		t.Error("rescan_enabled should be omitted when no --source is set")
	}
	if m["temperature_enabled"] != true {
		t.Error("temperature_enabled should be logged regardless of --source")
	}
}

func TestApplyRedactionOverrides_EmptyKeepsDefault(t *testing.T) {
	base := redaction.DefaultConfig()

	got, err := applyRedactionOverrides(base, config.RedactionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, base) {
		t.Errorf("empty overrides changed the config: got %+v, want %+v", got, base)
	}
}

func TestApplyRedactionOverrides_DisableDropsListedKeepsRest(t *testing.T) {
	base := redaction.DefaultConfig()

	o := config.RedactionConfig{DisableBuiltinKinds: []string{"jwt", "aws_access_key"}}
	got, err := applyRedactionOverrides(base, o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := make([]string, 0, len(base.BuiltinKinds))
	for _, k := range base.BuiltinKinds {
		if k != "jwt" && k != "aws_access_key" {
			want = append(want, k)
		}
	}
	if !reflect.DeepEqual(got.BuiltinKinds, want) {
		t.Errorf("BuiltinKinds = %v, want %v (disabled kinds removed, order preserved)", got.BuiltinKinds, want)
	}
}

func TestApplyRedactionOverrides_DisabledKindNotScrubbed(t *testing.T) {
	const awsKey = "AKIAIOSFODNN7EXAMPLE"
	const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkw.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	sample := "aws=" + awsKey + " jwt=" + jwt

	defR, err := redaction.New(redaction.DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defBaseline, _ := defR.Scrub(sample); strings.Contains(defBaseline, jwt) {
		t.Fatalf("jwt sample does not match the built-in detector; test would be vacuous: %q", defBaseline)
	}

	o := config.RedactionConfig{DisableBuiltinKinds: []string{"jwt"}}
	cfg, err := applyRedactionOverrides(redaction.DefaultConfig(), o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := redaction.New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, _ := r.Scrub(sample)

	if strings.Contains(out, awsKey) {
		t.Errorf("aws_access_key (not disabled) must still be redacted, got: %q", out)
	}
	if !strings.Contains(out, jwt) {
		t.Errorf("disabled jwt must pass through untouched, got: %q", out)
	}
}

func TestApplyRedactionOverrides_UnknownDisableKindFailsLoud(t *testing.T) {
	base := redaction.DefaultConfig()

	o := config.RedactionConfig{DisableBuiltinKinds: []string{"jwtt"}}
	if _, err := applyRedactionOverrides(base, o); err == nil {
		t.Fatal("expected error for unknown built-in kind")
	} else if !strings.Contains(err.Error(), "jwtt") {
		t.Errorf("error should name the unknown kind, got: %v", err)
	}
}

func TestApplyRedactionOverrides_CustomAppended(t *testing.T) {
	base := redaction.DefaultConfig()

	o := config.RedactionConfig{Custom: []config.RedactionPattern{{Kind: "internal_token", Pattern: "INT-[0-9a-f]{32}"}}}
	got, err := applyRedactionOverrides(base, o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []redaction.CustomPattern{{Kind: "internal_token", Pattern: "INT-[0-9a-f]{32}"}}
	if !reflect.DeepEqual(got.Custom, want) {
		t.Errorf("Custom = %v, want %v", got.Custom, want)
	}

	if !reflect.DeepEqual(got.BuiltinKinds, base.BuiltinKinds) {
		t.Error("custom patterns must not disturb the default built-in set")
	}
}

func TestApplyRedactionOverrides_InvalidCustomRegexFailsLoud(t *testing.T) {
	base := redaction.DefaultConfig()

	o := config.RedactionConfig{Custom: []config.RedactionPattern{{Kind: "bad_pattern", Pattern: "INT-([0-9a-f]{32}"}}}
	cfg, err := applyRedactionOverrides(base, o)
	if err != nil {
		t.Fatalf("merge should not fail; regex compiles in redaction.New: %v", err)
	}

	if _, err := redaction.New(cfg); err == nil {
		t.Fatal("expected error for invalid custom regex")
	} else if !strings.Contains(err.Error(), "bad_pattern") {
		t.Errorf("error should name the offending pattern kind, got: %v", err)
	}
}
