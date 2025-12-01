package validate

import (
	"fmt"

	"github.com/spf13/cobra"
)

func Args(cmd *cobra.Command, args []string, minArgs int, maxArgs int) (exit bool, _ error) {
	defer func() {
		if exit {
			// Disable normal run, if exit is requested.
			cmd.Run = func(cmd *cobra.Command, args []string) {}
			cmd.RunE = func(cmd *cobra.Command, args []string) error { return nil } //nolint:revive // https://github.com/mgechev/revive/issues/1528 hopefully fixed in one of the next golangci-lint versions.
		}
	}()

	if len(args) < minArgs || (maxArgs != -1 && len(args) > maxArgs) {
		_ = cmd.Help()

		if len(args) == 0 {
			return true, nil
		}

		return true, fmt.Errorf("Invalid number of arguments")
	}

	return false, nil
}
