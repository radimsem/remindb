package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	installShellURL = "https://raw.githubusercontent.com/radimsem/remindb/main/install.sh"
	installPSURL    = "https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Reinstall remindb by re-running the install script from main",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cobraCmd *cobra.Command, _ []string) error {
	cobraCmd.SilenceUsage = true

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
	return nil
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
