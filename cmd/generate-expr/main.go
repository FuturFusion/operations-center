package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:   "generate-expr",
		Short: "Expr-Lang compatible struct and converter generation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("Not implemented")
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}

	cmd.AddCommand(generateCmd())

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
