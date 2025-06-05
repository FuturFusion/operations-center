package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/cmds"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/config"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/version"
)

const (
	applicationName      = "operations-center"
	applicationEnvPrefix = "OPERATIONS_CENTER"
)

func main() {
	err := main0(os.Args[1:], os.Stdout, os.Stderr, environment.New(applicationName, applicationEnvPrefix))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type env interface {
	VarDir() string
	GetUnixSocket() string
	UserConfigDir() (string, error)
}

func main0(args []string, stdout io.Writer, stderr io.Writer, env env) error {
	app := &cobra.Command{}
	app.Use = applicationName
	app.Short = "Command line client for operations center"
	app.Long = `Description:
  Command line client for operations center

  The operations center can be interacted with through the various commands
  below. For help with any of those, simply call them with --help.
`

	app.SetArgs(args)
	app.SetOut(stdout)
	app.SetErr(stderr)

	app.SilenceUsage = true
	app.CompletionOptions = cobra.CompletionOptions{DisableDefaultCmd: true}
	app.SilenceErrors = true

	// Workaround for main command
	app.Args = cobra.ArbitraryArgs

	// Global flags
	globalCmd := cmdGlobal{
		cmd:      app,
		env:      env,
		config:   &config.Config{},
		ocClient: &client.OperationsCenterClient{},
	}

	app.PersistentPreRunE = globalCmd.Run
	app.PersistentFlags().BoolVar(&globalCmd.flagVersion, "version", false, "Print version number")
	app.PersistentFlags().BoolVarP(&globalCmd.flagHelp, "help", "h", false, "Print help")
	app.PersistentFlags().BoolVarP(&globalCmd.flagLogDebug, "debug", "d", false, "Show all debug messages")
	app.PersistentFlags().BoolVarP(&globalCmd.flagLogVerbose, "verbose", "v", false, "Show all information messages")
	app.PersistentFlags().BoolVar(&globalCmd.flagForceLocal, "force-local", false, "Force using the local unix socket")

	// Version handling
	app.SetVersionTemplate("{{.Version}}\n")
	app.Version = version.Version

	provisioningCmd := cmds.CmdProvisioning{
		OCClient: globalCmd.ocClient,
	}

	app.AddCommand(provisioningCmd.Command())

	inventoryCmd := cmds.CmdInventory{
		OCClient: globalCmd.ocClient,
	}

	app.AddCommand(inventoryCmd.Command())

	return app.Execute()
}

type cmdGlobal struct {
	cmd *cobra.Command
	env env

	config   *config.Config
	ocClient *client.OperationsCenterClient

	flagHelp    bool
	flagVersion bool

	flagLogDebug   bool
	flagLogVerbose bool

	flagForceLocal bool
}

func (c *cmdGlobal) Run(cmd *cobra.Command, args []string) error {
	err := logger.InitLogger(cmd.ErrOrStderr(), "", c.flagLogVerbose, c.flagLogDebug)
	if err != nil {
		return err
	}

	c.config.Verbose = c.flagLogVerbose
	c.config.Debug = c.flagLogDebug
	c.config.ForceLocal = c.flagForceLocal

	configDir, err := c.env.UserConfigDir()
	if err != nil {
		return err
	}

	err = os.MkdirAll(configDir, 0o700)
	if err != nil {
		return err
	}

	err = c.config.LoadConfig(configDir)
	if err != nil {
		return err
	}

	opts := []client.Option{}

	if c.config.ForceLocal {
		opts = append(opts, client.WithForceLocal(c.env.GetUnixSocket()))
	}

	if c.config.TLSClientCertFile != "" && c.config.TLSClientKeyFile != "" {
		opts = append(opts, client.WithClientCertificate(c.config.TLSClientCertFile, c.config.TLSClientKeyFile))
	}

	if c.config.AuthType == config.AuthTypeOIDC {
		opts = append(opts, client.WithOIDCTokensFile(filepath.Join(c.env.VarDir(), "oidc-tokens.json")))
	}

	*c.ocClient, err = client.New(
		c.config.OperationsCenterServer,
		opts...,
	)
	if err != nil {
		return err
	}

	return nil
}
