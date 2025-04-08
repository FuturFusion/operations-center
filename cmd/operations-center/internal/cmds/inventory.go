package cmds

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/cmds/inventory"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/config"
)

type CmdInventory struct {
	Config *config.Config
}

func (c *CmdInventory) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "inventory"
	cmd.Short = "Interact with inventory"
	cmd.Long = `Description:
  Interact with operations center inventory

  Configure inventory of operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Query
	queryCmd := inventory.CmdQuery{
		Config: c.Config,
	}

	cmd.AddCommand(queryCmd.Command())

	// Single resources
	imageCmd := inventory.CmdImage{
		Config: c.Config,
	}

	cmd.AddCommand(imageCmd.Command())

	instanceCmd := inventory.CmdInstance{
		Config: c.Config,
	}

	cmd.AddCommand(instanceCmd.Command())

	networkACLCmd := inventory.CmdNetworkACL{
		Config: c.Config,
	}

	cmd.AddCommand(networkACLCmd.Command())

	networkForwardCmd := inventory.CmdNetworkForward{
		Config: c.Config,
	}

	cmd.AddCommand(networkForwardCmd.Command())

	networkIntegrationCmd := inventory.CmdNetworkIntegration{
		Config: c.Config,
	}

	cmd.AddCommand(networkIntegrationCmd.Command())

	networkLoadBalancerCmd := inventory.CmdNetworkLoadBalancer{
		Config: c.Config,
	}

	cmd.AddCommand(networkLoadBalancerCmd.Command())

	networkPeerCmd := inventory.CmdNetworkPeer{
		Config: c.Config,
	}

	cmd.AddCommand(networkPeerCmd.Command())

	networkZoneCmd := inventory.CmdNetworkZone{
		Config: c.Config,
	}

	cmd.AddCommand(networkZoneCmd.Command())

	networkCmd := inventory.CmdNetwork{
		Config: c.Config,
	}

	cmd.AddCommand(networkCmd.Command())

	profileCmd := inventory.CmdProfile{
		Config: c.Config,
	}

	cmd.AddCommand(profileCmd.Command())

	projectCmd := inventory.CmdProject{
		Config: c.Config,
	}

	cmd.AddCommand(projectCmd.Command())

	storageBucketCmd := inventory.CmdStorageBucket{
		Config: c.Config,
	}

	cmd.AddCommand(storageBucketCmd.Command())

	storagePoolCmd := inventory.CmdStoragePool{
		Config: c.Config,
	}

	cmd.AddCommand(storagePoolCmd.Command())

	storageVolumeCmd := inventory.CmdStorageVolume{
		Config: c.Config,
	}

	cmd.AddCommand(storageVolumeCmd.Command())

	return cmd
}
