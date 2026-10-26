package patches

import (
	"context"
	"os"
	"strings"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
)

type patchStage int

// Define the stages that patches can run at.
const (
	noStageSet patchStage = iota
	PreSecurityInfrastructure
)

/*
Patches are one-time actions that are sometimes needed to update:

  - move things around on the filesystem
  - migrate settings in the configuration

Those patches are applied at startup time after the database schema
has been fully updated. Patches can therefore assume a working database.

DO NOT use this mechanism for database update. Schema updates must be
done through the separate schema update mechanism.

Only append to the patches list, never remove entries and never re-order them.
*/
var patches = []patch{
	{name: "rename_channels_to_upstream_channels", stage: PreSecurityInfrastructure, run: patchRenameChannelsToUpstreamChannels},
	{name: "remove_var_lib_operations_center_terraform_dirs", stage: PreSecurityInfrastructure, run: patchRemoveVarLibOperationCenterTerraformDirs},
}

func patchRenameChannelsToUpstreamChannels(ctx context.Context, name string) error {
	updatesCfg := config.GetUpdates()

	updatesCfg.FilterExpression = strings.ReplaceAll(updatesCfg.FilterExpression, "channels", "upstream_channels")
	return config.UpdateUpdates(ctx, updatesCfg.UpdatesPut)
}

func patchRemoveVarLibOperationCenterTerraformDirs(ctx context.Context, name string) error {
	err := os.RemoveAll("/var/lib/operations-center/servercerts")
	if err != nil {
		return err
	}

	return os.RemoveAll("/var/lib/operations-center/terraform")
}
