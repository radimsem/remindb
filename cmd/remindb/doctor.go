package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/radimsem/remindb/pkg/doctor"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/spf13/cobra"
)

var (
	doctorJSON bool
	doctorFix  bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run integrity checks against the database (use --fix to self-heal)",
	RunE:  runDoctor,
}

// errDoctorFailures is the sentinel that maps to a non-zero exit without printing a usage banner.
var errDoctorFailures = errors.New("doctor: one or more checks failed")

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Emit machine-readable JSON")
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Attempt to repair failed checks (takes a timestamped backup first)")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	report, err := dispatchDoctor(ctx, st)
	if err != nil {
		return err
	}

	if err := emit(os.Stdout, report); err != nil {
		return err
	}

	if report.HasFailures() {
		return errDoctorFailures
	}
	return nil
}

func dispatchDoctor(ctx context.Context, st *store.Store) (doctor.Report, error) {
	if !doctorFix {
		return doctor.Run(ctx, st), nil
	}

	pre := doctor.Run(ctx, st)
	if pre.HasFailures() {
		// UnixNano avoids collision when --fix runs more than once per wall-clock second.
		backup := fmt.Sprintf("%s.bak.%d", dbPath, time.Now().UnixNano())
		if err := st.BackupTo(ctx, backup); err != nil {
			return doctor.Report{}, err
		}

		_, _ = fmt.Fprintf(os.Stderr, "doctor: backup written to %s\n", backup)
	}

	return doctor.Heal(ctx, st), nil
}

func emit(w io.Writer, r doctor.Report) error {
	if doctorJSON {
		return r.WriteJSON(w)
	}
	color := isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == ""
	return r.WriteText(w, color)
}
