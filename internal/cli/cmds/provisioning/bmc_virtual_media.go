package provisioning

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Attach installation media to a server via BMC.
type cmdServerBMCAttachMedia struct {
	ocClient *client.OperationsCenterClient

	flagType          string
	flagArchitecture  string
	flagChannel       string
	flagSetBootDevice bool
}

func (c *cmdServerBMCAttachMedia) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "attach-media <name> <virtual-media-id> <token-uuid> <seed>"
	cmd.Short = "Attach installation media to a server via BMC"
	cmd.Long = `Description:
  Attach installation media to a server via BMC.

  The installation media is generated from a public token seed (the same image
  served by the "token seed get-image" command) and attached to the given
  virtual media device as a virtual CD/DVD. The BMC streams the image directly
  from Operations Center, so the token seed must be public.

  The virtual media device is selected by its ID in the "<service>:<id>"
  notation (e.g. "system:1" or "manager:2"), as reported in the server's BMC
  virtual media data.

  With --set-boot-device, the virtual media is in addition registered as the
  boot device for the next boot of the server, so the server boots the attached
  installation media without changing the persistent boot order. Detaching the
  media restores the default boot configuration of the system.
`

	cmd.Flags().StringVar(&c.flagType, "type", "iso", "type of image (iso|raw)")
	cmd.Flags().StringVar(&c.flagArchitecture, "architecture", "x86_64", "CPU architecture for the image (x86_64|aarch64)")
	cmd.Flags().StringVar(&c.flagChannel, "channel", "", "Channel, the most recent update should be taken from to generate the image")
	cmd.Flags().BoolVar(&c.flagSetBootDevice, "set-boot-device", false, "Register the virtual media as the boot device for the next boot")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCAttachMedia) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 4, 4)
	if exit {
		return err
	}

	return validateImageTypeAndArchitecture(cmd.Flag("type").Value.String(), cmd.Flag("architecture").Value.String())
}

func (c *cmdServerBMCAttachMedia) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	virtualMediaID := args[1]
	tokenUUID := args[2]
	seed := args[3]

	err := c.ocClient.BMCAttachMedia(cmd.Context(), name, api.ServerBMCAttachMedia{
		TokenUUID:      tokenUUID,
		Seed:           seed,
		Type:           c.flagType,
		Architecture:   c.flagArchitecture,
		Channel:        c.flagChannel,
		VirtualMediaID: virtualMediaID,
		SetBootDevice:  c.flagSetBootDevice,
	})
	if err != nil {
		return err
	}

	return nil
}

// Detach installation media from a server via BMC.
type cmdServerBMCDetachMedia struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCDetachMedia) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "detach-media <name> <virtual-media-id>"
	cmd.Short = "Detach installation media from a server via BMC"
	cmd.Long = `Description:
  Detach installation media from a server via BMC.

  The virtual media device is selected by its ID in the "<service>:<id>"
  notation (e.g. "system:1" or "manager:2"), as reported in the server's BMC
  virtual media data.

  If the server is currently set to boot from the detached virtual media, the
  default boot configuration of the system is restored as well.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCDetachMedia) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCDetachMedia) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	virtualMediaID := args[1]

	err := c.ocClient.BMCDetachMedia(cmd.Context(), name, api.ServerBMCDetachMedia{
		VirtualMediaID: virtualMediaID,
	})
	if err != nil {
		return err
	}

	return nil
}
