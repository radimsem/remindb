package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

const (
	githubLatestURL = "https://api.github.com/repos/radimsem/remindb/releases/latest"
	versionCheckTTL = 5 * time.Second
)

var version = "dev"

func init() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if v := info.Main.Version; v != "" && v != "(devel)" {
				version = v
			}
		}
	}
	rootCmd.Version = version
}

func checkLatestVersion(ctx context.Context, current string, logger *slog.Logger) {
	if !strings.HasPrefix(current, "v") {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, versionCheckTTL)
	defer cancel()

	latest, err := fetchLatestTag(ctx)
	if err != nil {
		logger.Debug("version check skipped", "err", err)
		return
	}

	if latest != "" && latest != current {
		logger.Info("newer version available",
			"current", current,
			"latest", latest,
			"hint", "remindb update",
		)
	}
}

func fetchLatestTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("non-OK status: %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to decode: %w", err)
	}

	return release.TagName, nil
}
