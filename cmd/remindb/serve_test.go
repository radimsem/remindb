package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/redaction"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/temperature"
)

func ptr[T any](v T) *T { return &v }

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
