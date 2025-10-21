package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lxc/incus/v6/shared/ask"
	"github.com/lxc/incus/v6/shared/termios"
	localtls "github.com/lxc/incus/v6/shared/tls"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/cmds"
	"github.com/FuturFusion/operations-center/internal/client"
	config "github.com/FuturFusion/operations-center/internal/config/cli"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/version"
)

func main() {
	err := main0(os.Args[1:], os.Stdout, os.Stderr, environment.New(config.ApplicationName, config.ApplicationEnvPrefix))
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
	app.Use = config.ApplicationName
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

	app.PersistentPreRunE = globalCmd.PreRun
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

	remoteCmd := cmds.CmdRemote{
		Env: env,
	}

	app.AddCommand(remoteCmd.Command())

	systemCmd := cmds.CmdSystem{
		OCClient: globalCmd.ocClient,
	}

	app.AddCommand(systemCmd.Command())

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

func (c *cmdGlobal) PreRun(cmd *cobra.Command, args []string) error {
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

	err = c.config.LoadConfig(configDir)
	if err != nil {
		return err
	}

	opts := []client.Option{}

	// Use local unix socket connection by default.
	remote := config.Remote{
		Addr:     "http://unix.socket",
		AuthType: config.AuthTypeUntrusted,
	}

	if c.config.DefaultRemote != "" {
		r, ok := c.config.Remotes[c.config.DefaultRemote]
		if ok {
			remote = r
		} else {
			cmd.PrintErrf("Warning: configured default remote %q does not exist, fallback to local unix socket\n", c.config.DefaultRemote)
		}
	}

	if c.config.ForceLocal || c.config.DefaultRemote == "" {
		opts = append(opts, client.WithForceLocal(c.env.GetUnixSocket()))
	}

	opts = append(opts, client.WithClientCertificate(c.config.CertInfo))

	if remote.AuthType == config.AuthTypeOIDC {
		opts = append(opts, client.WithOIDCTokensFile(filepath.Join(configDir, "oidc-tokens", c.config.DefaultRemote+".json")))
	}

	*c.ocClient, err = client.New(
		remote.Addr,
		opts...,
	)
	if err != nil {
		return err
	}

	// Skip verification of remote certificate and authentication if unix socket.
	if c.config.ForceLocal || c.config.DefaultRemote == "" {
		return nil
	}

	serverCert, ok, err := c.ocClient.IsServerTrusted(cmd.Context(), remote.ServerCert)
	if err != nil {
		return err
	}

	if !ok {
		// If we don't have a regular terminal, fail immediately, since we can not
		// ask the user for manual verification.
		if !termios.IsTerminal(environment.GetStdinFd()) {
			return fmt.Errorf("Aborting due to untrusted server TLS certificate")
		}

		// asker to verify fingerprint
		asker := ask.NewAsker(bufio.NewReader(cmd.InOrStdin()))
		trustedCert, err := asker.AskBool(fmt.Sprintf("Server presented an untrusted TLS certificate with SHA256 fingerprint %s. Is this the correct fingerprint? (yes/no) [default=no]: ", localtls.CertFingerprint(serverCert.Certificate)), "no")
		if err != nil {
			return err
		}

		if !trustedCert {
			return fmt.Errorf("Aborting due to untrusted server TLS certificate")
		}

		remote.ServerCert = serverCert
		if c.config.DefaultRemote != "" {
			c.config.Remotes[c.config.DefaultRemote] = remote
		}

		// Run SaveConfig to store the server cert in case it changed.
		err = c.config.SaveConfig()
		if err != nil {
			return err
		}
	}

	return nil
}
