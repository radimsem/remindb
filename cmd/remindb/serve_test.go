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
	"github.com/radimsem/remindb/pkg/temperature"
	"github.com/spf13/cobra"
)

func ptr[T any](v T) *T { return &v }

func newServeTestCmd(t *testing.T) *cobra.Command {
	t.Helper()

	c := &cobra.Command{Use: "serve"}
	c.Flags().StringVar(&transport, "transport", remindb.TransportStdio, "")
	c.Flags().StringVar(&listen, "listen", remindb.DefaultListenAddr, "")

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
	lg, file, err := newServeLogger(false, config.LoggingConfig{Level: ptr("warn")})
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
	lg, _, err := newServeLogger(true, config.LoggingConfig{Level: ptr("error")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !lg.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("--verbose must force debug even when config says error")
	}
}

func TestNewServeLogger_JsonFileOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.log")

	lg, file, err := newServeLogger(false, config.LoggingConfig{Format: ptr("json"), OutputPath: ptr(path)})
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

	_, _, err := newServeLogger(false, config.LoggingConfig{OutputPath: ptr(bad)})
	if err == nil {
		t.Fatal("expected loud failure when output_path cannot be opened")
	}
	if !strings.Contains(err.Error(), bad) {
		t.Errorf("error should name the unopenable path, got: %v", err)
	}
}

func TestApplyTemperatureOverrides_Empty(t *testing.T) {
	base := temperature.DefaultConfig()

	got := applyTemperatureOverrides(base, config.TemperatureConfig{})
	if got != base {
		t.Errorf("empty overrides changed the config: got %+v, want %+v", got, base)
	}
}

func TestApplyTemperatureOverrides_AllFields(t *testing.T) {
	base := temperature.DefaultConfig()

	o := config.TemperatureConfig{
		DecayRate:        ptr(0.03),
		AccessBoost:      ptr(0.2),
		ColdThreshold:    ptr(0.08),
		NotifyThreshold:  ptr(0.07),
		SummarizeRebound: ptr(0.6),
		TickInterval:     ptr(config.Duration(10 * time.Minute)),
		ColdNotifyTTL:    ptr(config.Duration(2 * time.Hour)),
		ColdNotifyLimit:  ptr(100),
	}

	got := applyTemperatureOverrides(base, o)

	want := temperature.Config{
		DecayRate:        0.03,
		AccessBoost:      0.2,
		ColdThreshold:    0.08,
		NotifyThreshold:  0.07,
		SummarizeRebound: 0.6,
		TickInterval:     10 * time.Minute,
		ColdNotifyTTL:    2 * time.Hour,
		ColdNotifyLimit:  100,
	}
	if got != want {
		t.Errorf("applyTemperatureOverrides = %+v, want %+v", got, want)
	}
}

func TestApplyTemperatureOverrides_PartialKeepsBase(t *testing.T) {
	base := temperature.DefaultConfig()

	got := applyTemperatureOverrides(base, config.TemperatureConfig{DecayRate: ptr(0.99)})

	if got.DecayRate != 0.99 {
		t.Errorf("DecayRate = %g, want 0.99 (overridden)", got.DecayRate)
	}
	if got.AccessBoost != base.AccessBoost {
		t.Errorf("AccessBoost = %g, want %g (untouched default)", got.AccessBoost, base.AccessBoost)
	}
	if got.TickInterval != base.TickInterval {
		t.Errorf("TickInterval = %s, want %s (untouched default)", got.TickInterval, base.TickInterval)
	}
}

// A zero override must overwrite the default — the reason fields are pointers.
func TestApplyTemperatureOverrides_ExplicitZeroOverrides(t *testing.T) {
	base := temperature.DefaultConfig()

	got := applyTemperatureOverrides(base, config.TemperatureConfig{DecayRate: ptr(0.0)})

	if got.DecayRate != 0 {
		t.Errorf("DecayRate = %g, want 0 (explicit zero must override default %g)", got.DecayRate, base.DecayRate)
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
