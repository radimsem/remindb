// Package config loads workspace-level remindb configuration from .remindb/config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DirName  = ".remindb"
	FileName = "config.json"
	Path     = DirName + "/" + FileName
)

type Config struct {
	Budgets     BudgetsConfig     `json:"budgets"`
	Compile     CompileConfig     `json:"compile"`
	Redaction   RedactionConfig   `json:"redaction"`
	Server      ServerConfig      `json:"server"`
	Temperature TemperatureConfig `json:"temperature"`
}

type ServerConfig struct {
	Transport *string       `json:"transport,omitempty"`
	Listen    *string       `json:"listen,omitempty"`
	Logging   LoggingConfig `json:"logging,omitempty"`
}

type LoggingConfig struct {
	Level      *string `json:"level,omitempty"`
	Format     *string `json:"format,omitempty"`
	OutputPath *string `json:"output_path,omitempty"`
}

type BudgetsConfig struct {
	Search     *int `json:"search,omitempty"`
	Fetch      *int `json:"fetch,omitempty"`
	FetchBatch *int `json:"fetch_batch,omitempty"`
	Related    *int `json:"related,omitempty"`
}

type CompileConfig struct {
	MaxFileSize      *ByteSize `json:"max_file_size,omitempty"`
	MaxParallelism   *int      `json:"max_parallelism,omitempty"`
	WallClockTimeout *Duration `json:"wall_clock_timeout,omitempty"`
}

type RedactionConfig struct {
	DisableBuiltinKinds []string           `json:"disable_builtin_kinds,omitempty"`
	Custom              []RedactionPattern `json:"custom,omitempty"`
}

type RedactionPattern struct {
	Kind    string `json:"kind"`
	Pattern string `json:"pattern"`
}

type TemperatureConfig struct {
	DecayRate        *float64  `json:"decay_rate,omitempty"`
	AccessBoost      *float64  `json:"access_boost,omitempty"`
	ColdThreshold    *float64  `json:"cold_threshold,omitempty"`
	NotifyThreshold  *float64  `json:"notify_threshold,omitempty"`
	SummarizeRebound *float64  `json:"summarize_rebound,omitempty"`
	TickInterval     *Duration `json:"tick_interval,omitempty"`
	ColdNotifyTTL    *Duration `json:"cold_notify_ttl,omitempty"`
	ColdNotifyLimit  *int      `json:"cold_notify_limit,omitempty"`
}

type Duration time.Duration

// Accept a duration string ("10m", "2h"); JSON has no native duration form.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}

	*d = Duration(parsed)
	return nil
}

type ByteSize int64

// Unit suffixes, longest-first so "2GB" matches "GB" before "B".
var byteUnits = []struct {
	suffix string
	mult   int64
}{
	{"KIB", 1 << 10},
	{"MIB", 1 << 20},
	{"GIB", 1 << 30},
	{"TIB", 1 << 40},
	{"KB", 1 << 10},
	{"MB", 1 << 20},
	{"GB", 1 << 30},
	{"TB", 1 << 40},
	{"K", 1 << 10},
	{"M", 1 << 20},
	{"G", 1 << 30},
	{"T", 1 << 40},
	{"B", 1},
}

func (s *ByteSize) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}

	n, err := parseByteSize(str)
	if err != nil {
		return fmt.Errorf("invalid size %q: %w", str, err)
	}

	*s = ByteSize(n)
	return nil
}

func parseByteSize(s string) (int64, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	if t == "" {
		return 0, errors.New("empty size")
	}

	num, mult := t, int64(1)
	for _, u := range byteUnits {
		if strings.HasSuffix(t, u.suffix) {
			num = strings.TrimSpace(t[:len(t)-len(u.suffix)])
			mult = u.mult

			break
		}
	}

	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, errors.New("not a number")
	}
	if f < 0 {
		return 0, errors.New("must not be negative")
	}

	return int64(f * float64(mult)), nil
}

// Read <workspace>/.remindb/config.json. Missing or empty → zero-value Config.
func Load(workspace string) (Config, error) {
	f, err := os.Open(filepath.Join(workspace, DirName, FileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("failed to read: %s: %w", Path, err)
	}
	defer func() { _ = f.Close() }()

	var cfg Config
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return Config{}, nil
		}

		if key, ok := unknownFieldKey(err); ok {
			return Config{}, fmt.Errorf("unknown key %q in %s", key, Path)
		}
		return Config{}, fmt.Errorf("failed to parse: %s: %w", Path, err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", Path, err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	cc := c.Compile

	if cc.MaxFileSize != nil && *cc.MaxFileSize <= 0 {
		return errors.New("compile.max_file_size must be positive")
	}
	if cc.MaxParallelism != nil && *cc.MaxParallelism < 1 {
		return errors.New("compile.max_parallelism must be >= 1")
	}
	if cc.WallClockTimeout != nil && *cc.WallClockTimeout < 0 {
		return errors.New("compile.wall_clock_timeout must not be negative")
	}

	bc := c.Budgets

	if bc.Search != nil && *bc.Search <= 0 {
		return errors.New("budgets.search must be positive")
	}
	if bc.Fetch != nil && *bc.Fetch <= 0 {
		return errors.New("budgets.fetch must be positive")
	}
	if bc.FetchBatch != nil && *bc.FetchBatch <= 0 {
		return errors.New("budgets.fetch_batch must be positive")
	}
	if bc.Related != nil && *bc.Related <= 0 {
		return errors.New("budgets.related must be positive")
	}

	sc := c.Server

	if sc.Transport != nil {
		switch *sc.Transport {
		case "stdio", "http":
		default:
			return fmt.Errorf("server.transport must be \"stdio\" or \"http\", got %q", *sc.Transport)
		}
	}

	lg := sc.Logging

	if lg.Level != nil {
		switch *lg.Level {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("server.logging.level must be one of debug|info|warn|error, got %q", *lg.Level)
		}
	}
	if lg.Format != nil {
		switch *lg.Format {
		case "text", "json":
		default:
			return fmt.Errorf("server.logging.format must be \"text\" or \"json\", got %q", *lg.Format)
		}
	}

	return nil
}

func unknownFieldKey(err error) (string, bool) {
	const prefix = `json: unknown field "`

	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		return "", false
	}

	key, _, found := strings.Cut(msg[len(prefix):], `"`)
	if !found {
		return "", false
	}
	return key, true
}
