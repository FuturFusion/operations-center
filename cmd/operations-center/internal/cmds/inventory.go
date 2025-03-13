package cmds

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/cmds/inventory"
)

type CmdInventory struct{}

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

	imageCmd := inventory.CmdImage{}
	cmd.AddCommand(imageCmd.Command())

	instanceCmd := inventory.CmdInstance{}
	cmd.AddCommand(instanceCmd.Command())

	networkACLCmd := inventory.CmdNetworkACL{}
	cmd.AddCommand(networkACLCmd.Command())

	networkForwardCmd := inventory.CmdNetworkForward{}
	cmd.AddCommand(networkForwardCmd.Command())

	networkIntegrationCmd := inventory.CmdNetworkIntegration{}
	cmd.AddCommand(networkIntegrationCmd.Command())

	networkLoadBalancerCmd := inventory.CmdNetworkLoadBalancer{}
	cmd.AddCommand(networkLoadBalancerCmd.Command())

	networkPeerCmd := inventory.CmdNetworkPeer{}
	cmd.AddCommand(networkPeerCmd.Command())

	networkZoneCmd := inventory.CmdNetworkZone{}
	cmd.AddCommand(networkZoneCmd.Command())

	networkCmd := inventory.CmdNetwork{}
	cmd.AddCommand(networkCmd.Command())

	profileCmd := inventory.CmdProfile{}
	cmd.AddCommand(profileCmd.Command())

	projectCmd := inventory.CmdProject{}
	cmd.AddCommand(projectCmd.Command())

	storageBucketCmd := inventory.CmdStorageBucket{}
	cmd.AddCommand(storageBucketCmd.Command())

	storagePoolCmd := inventory.CmdStoragePool{}
	cmd.AddCommand(storagePoolCmd.Command())

	storageVolumeCmd := inventory.CmdStorageVolume{}
	cmd.AddCommand(storageVolumeCmd.Command())

	return cmd
}
