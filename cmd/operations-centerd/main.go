package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/version"
)

const (
	applicationName      = "operations-centerd"
	applicationEnvPrefix = "OPERATIONS_CENTER"
)

func main() {
	err := main0(os.Args[1:], os.Stdout, os.Stderr, environment.New(applicationName, applicationEnvPrefix))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main0(args []string, stdout io.Writer, stderr io.Writer, env env) error {
	defaultLogFile := filepath.Join(env.LogDir(), applicationName+".log")

	// daemon command (main)
	daemonCmd := cmdDaemon{
		env: env,
	}

	app := daemonCmd.Command()
	app.SetArgs(args)
	app.SetOut(stdout)
	app.SetErr(stderr)

	app.SilenceUsage = true
	app.CompletionOptions = cobra.CompletionOptions{DisableDefaultCmd: true}
	app.SilenceErrors = true

	// Workaround for main command
	app.Args = cobra.ArbitraryArgs

	// Global flags
	globalCmd := cmdGlobal{cmd: app}
	app.PersistentPreRunE = globalCmd.Run
	app.PersistentFlags().BoolVar(&globalCmd.flagVersion, "version", false, "Print version number")
	app.PersistentFlags().BoolVarP(&globalCmd.flagHelp, "help", "h", false, "Print help")
	app.PersistentFlags().StringVar(&globalCmd.flagLogFile, "logfile", defaultLogFile, "Path to the log file")
	app.PersistentFlags().BoolVarP(&globalCmd.flagLogDebug, "debug", "d", false, "Show all debug messages")
	app.PersistentFlags().BoolVarP(&globalCmd.flagLogVerbose, "verbose", "v", false, "Show all information messages")

	// Version handling
	app.SetVersionTemplate("{{.Version}}\n")
	app.Version = version.Version

	// Run the main command
	return app.Execute()
}

type cmdGlobal struct {
	cmd *cobra.Command

	flagHelp    bool
	flagVersion bool

	flagLogFile    string
	flagLogDebug   bool
	flagLogVerbose bool
}

func (c *cmdGlobal) Run(cmd *cobra.Command, args []string) error {
	err := logger.InitLogger(cmd.ErrOrStderr(), c.flagLogFile, c.flagLogVerbose, c.flagLogDebug)
	if err != nil {
		return err
	}

	return nil
}
