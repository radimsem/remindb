package main

import (
	"context"
	"fmt"
	"os"

	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/spf13/cobra"
)

var (
	compileMsg         string
	compileReseedTemps bool
)

var compileCmd = &cobra.Command{
	Use:   "compile <path>",
	Short: "One-shot compilation of files or a directory into the database",
	Args:  cobra.ExactArgs(1),
	RunE:  runCompile,
}

func init() {
	compileCmd.Flags().StringVarP(&compileMsg, "message", "m", "", "Snapshot message")
	compileCmd.Flags().BoolVar(&compileReseedTemps, "reseed-temperatures", false, "Override stored temperatures with .temp.json values on unchanged nodes (directory compiles only)")
	rootCmd.AddCommand(compileCmd)
}

func runCompile(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	path := args[0]

	if err := deriveDefaultDBPath(cmd, path); err != nil {
		return err
	}

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	msg := compileMsg
	if msg == "" {
		msg = "compile:" + path
	}

	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat: %w", err)
	}

	var result *compiler.Result
	if fi.IsDir() {
		var dirOpts []compiler.Option
		if compileReseedTemps {
			dirOpts = append(dirOpts, compiler.WithReseedTemperatures())
		}

		result, err = compiler.CompileDir(ctx, st, path, msg, dirOpts...)
	} else {
		result, err = compiler.CompileFile(ctx, st, path, msg)
	}
	if err != nil {
		return fmt.Errorf("failed to compile: %s: %w", path, err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "compiled: %d added, %d modified, %d removed (%d ops)\n",
		result.Added, result.Modified, result.Removed, result.Total)

	return nil
}
