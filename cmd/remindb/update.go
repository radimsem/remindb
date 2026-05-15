package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/radimsem/remindb/pkg/version"
)

const (
	installShellURL = "https://raw.githubusercontent.com/radimsem/remindb/main/install.sh"
	installPSURL    = "https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1"

	skillsUpdateHint = `Run "npx skills@latest update" to refresh agent-side skills.`
)

var updateForce bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Reinstall remindb by re-running the install script from main",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Reinstall even when the installed version matches the latest release")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cobraCmd *cobra.Command, _ []string) error {
	cobraCmd.SilenceUsage = true

	if !updateForce && alreadyOnLatest(cobraCmd.Context()) {
		fmt.Fprintln(os.Stderr, "Already up to date.")
		fmt.Fprintln(os.Stderr, skillsUpdateHint)
		return nil
	}

	cmd, err := installCommand()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "running: %s\n", cmd.String())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	fmt.Fprintln(os.Stderr, skillsUpdateHint)
	return nil
}

func alreadyOnLatest(ctx context.Context) bool {
	current := version.Get()
	if !strings.HasPrefix(current, "v") {
		return false
	}
	if i := strings.Index(current, "+"); i >= 0 {
		current = current[:i]
	}

	ctx, cancel := context.WithTimeout(ctx, versionCheckTTL)
	defer cancel()

	latest, err := fetchLatestTag(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "version check skipped: %v\n", err)
		return false
	}
	return latest == current
}

func installCommand() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("powershell", "-Command", "iwr -useb "+installPSURL+" | iex"), nil
	case "linux", "darwin":
		return exec.Command("bash", "-c", "curl -fsSL "+installShellURL+" | bash"), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
