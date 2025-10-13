package cmds

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	config "github.com/FuturFusion/operations-center/internal/config/cli"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
)

type environment interface {
	UserConfigDir() (string, error)
}

type CmdRemote struct {
	Env environment
}

func (c *CmdRemote) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remote"
	cmd.Short = "Manage the list of remote operations centers"
	cmd.Long = `Description:
  Manage the list of remote operations centers
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Add
	remoteAddCmd := cmdRemoteAdd{
		env: c.Env,
	}

	cmd.AddCommand(remoteAddCmd.Command())

	// List
	remoteListCmd := cmdRemoteList{
		env: c.Env,
	}

	cmd.AddCommand(remoteListCmd.Command())

	// Remove
	remoteRemoveCmd := cmdRemoteRemove{
		env: c.Env,
	}

	cmd.AddCommand(remoteRemoveCmd.Command())

	// Switch
	remoteSwitchCmd := cmdRemoteSwitch{
		env: c.Env,
	}

	cmd.AddCommand(remoteSwitchCmd.Command())

	return cmd
}

// Add remote.
type cmdRemoteAdd struct {
	env environment

	authType          string
	tlsClientCertFile string
	tlsClientKeyFile  string
}

func (c *cmdRemoteAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name> <URL>"
	cmd.Short = "Add a new remote"
	cmd.Long = `Description:
  Add a new remote

  Adds a new remote operations center.
`

	cmd.Flags().StringVar(&c.authType, "auth-type", "tls", "Server authentication type (tls or oidc)")
	cmd.Flags().StringVar(&c.tlsClientCertFile, "cert", "", "Path to TLS client certificate file in PEM format")
	cmd.Flags().StringVar(&c.tlsClientKeyFile, "key", "", "Path to TLS client key file in PEM format")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdRemoteAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	name := args[0]
	addr := args[1]

	if name == "" {
		return fmt.Errorf(`Name of remote can not be empty`)
	}

	if name == "local" {
		return fmt.Errorf(`Name of remote can not be "local", since it is a reserved name for the local access through unix socket`)
	}

	addrURL, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf(`Provided URL %q is not valid: %v`, addr, err)
	}

	if addrURL.Scheme != "https" {
		return fmt.Errorf(`Provided URL %q is not valid: protocol scheme needs to be https`, addr)
	}

	if config.AuthType(c.authType) != config.AuthTypeTLS && config.AuthType(c.authType) != config.AuthTypeOIDC {
		return fmt.Errorf(`Value for flag "--auth-type" needs to be %q or %q`, config.AuthTypeTLS, config.AuthTypeOIDC)
	}

	return nil
}

func (c *cmdRemoteAdd) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	addr := args[1]

	cfg := config.Config{}

	configDir, err := c.env.UserConfigDir()
	if err != nil {
		return err
	}

	err = cfg.LoadConfig(configDir)
	if err != nil {
		return err
	}

	_, ok := cfg.Remotes[name]
	if ok {
		return fmt.Errorf(`Remote with name %q already exists`, name)
	}

	if config.AuthType(c.authType) == config.AuthTypeTLS &&
		(cfg.TLSClientCertFile == "" || cfg.TLSClientKeyFile == "") &&
		(c.tlsClientCertFile == "" || c.tlsClientKeyFile == "") {
		return fmt.Errorf(`No TLS client certificate or key configured yet, please provide with "--cert" and "--key"`)
	}

	if config.AuthType(c.authType) == config.AuthTypeOIDC {
		remoteOCClient, err := client.New(addr, client.WithOIDCTokensFile(filepath.Join(configDir, "oidc-tokens", name+".json")))
		if err != nil {
			return fmt.Errorf(`Failed to create client for new remote: %v`, err)
		}

		_, err = remoteOCClient.GetClusters(cmd.Context())
		if err != nil {
			return fmt.Errorf(`Failed to connect to new remote: %v`, err)
		}
	}

	if cfg.Remotes == nil {
		cfg.Remotes = map[string]config.Remote{}
	}

	cfg.Remotes[name] = config.Remote{
		Addr:     addr,
		AuthType: config.AuthType(c.authType),
	}

	if c.tlsClientCertFile != "" && c.tlsClientKeyFile != "" {
		cfg.TLSClientCertFile = c.tlsClientCertFile
		cfg.TLSClientKeyFile = c.tlsClientKeyFile
	}

	err = cfg.SaveConfig(configDir)
	if err != nil {
		return fmt.Errorf(`Failed to update client config: %v`, err)
	}

	return nil
}

// List remotes.
type cmdRemoteList struct {
	env environment

	flagFormat string
}

func (c *cmdRemoteList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available remotes"
	cmd.Long = `Description:
  List the available remotes
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdRemoteList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdRemoteList) run(cmd *cobra.Command, args []string) error {
	cfg := config.Config{}

	configDir, err := c.env.UserConfigDir()
	if err != nil {
		return err
	}

	err = cfg.LoadConfig(configDir)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Address", "Auth Type"}
	data := [][]string{}
	localName := "local"
	if cfg.DefaultRemote == "" {
		localName = "local (current)"
	}

	data = append(data, []string{localName, "unix://", "file access"})

	for name, remote := range cfg.Remotes {
		if name == cfg.DefaultRemote {
			name += " (current)"
		}

		data = append(data, []string{name, remote.Addr, string(remote.AuthType)})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // Name
			Less:  sort.NaturalLess,
		},
		{
			Index: 1, // Address
			Less:  sort.NaturalLess,
		},
		{
			Index: 2, // Auth type
			Less:  sort.NaturalLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, cfg.Remotes)
}

// Remove remote.
type cmdRemoteRemove struct {
	env environment
}

func (c *cmdRemoteRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a remote"
	cmd.Long = `Description:
  Remove a remote

  Removes a remote operations center.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdRemoteRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	if name == "local" {
		return fmt.Errorf(`Remote "local" can not be remove, it does always exist explicitly`)
	}

	return nil
}

func (c *cmdRemoteRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg := config.Config{}

	configDir, err := c.env.UserConfigDir()
	if err != nil {
		return err
	}

	err = cfg.LoadConfig(configDir)
	if err != nil {
		return err
	}

	remote, ok := cfg.Remotes[name]
	if !ok {
		return fmt.Errorf(`Remote with name %q does not exist`, name)
	}

	delete(cfg.Remotes, name)

	if cfg.DefaultRemote == name {
		cfg.DefaultRemote = ""
	}

	if remote.AuthType == config.AuthTypeOIDC {
		err = os.Remove(filepath.Join(configDir, "oidc-tokens", name+".json"))
		if err != nil {
			cmd.PrintErrf("Warning: Failed to remove oidc tokens file: %v\n", err)
		}
	}

	err = cfg.SaveConfig(configDir)
	if err != nil {
		return fmt.Errorf(`Failed to update client config: %v`, err)
	}

	return nil
}

// Switch remote.
type cmdRemoteSwitch struct {
	env environment
}

func (c *cmdRemoteSwitch) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "switch <name>"
	cmd.Short = "Switch remote"
	cmd.Long = `Description:
  Switch remote

  Switches the default remote operations center that is interacted with.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdRemoteSwitch) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdRemoteSwitch) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	if name == "local" {
		name = ""
	}

	cfg := config.Config{}

	configDir, err := c.env.UserConfigDir()
	if err != nil {
		return err
	}

	err = cfg.LoadConfig(configDir)
	if err != nil {
		return err
	}

	_, ok := cfg.Remotes[name]
	if !ok && name != "" {
		return fmt.Errorf(`Remote with name %q does not exist`, name)
	}

	cfg.DefaultRemote = name

	err = cfg.SaveConfig(configDir)
	if err != nil {
		return fmt.Errorf(`Failed to update client config: %v`, err)
	}

	return nil
}
