package provisioning

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lxc/incus/v7/shared/units"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
)

// deployStatusPollInterval is how often the deployment status is queried while
// waiting for a deployment to finish.
var deployStatusPollInterval = 5 * time.Second

// Deploy IncusOS on a server.
type cmdServerDeploy struct {
	ocClient *client.OperationsCenterClient

	flagVirtualMediaID             string
	flagType                       string
	flagArchitecture               string
	flagChannel                    string
	flagForce                      bool
	flagSkipSecureBootCertificates bool
	flagWait                       bool
}

func (c *cmdServerDeploy) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "deploy <name> <token-uuid> <seed>"
	cmd.Short = "Deploy IncusOS on a server"
	cmd.Long = `Description:
  Deploy IncusOS on a pre-registered server.

  Operations Center configures the BIOS of the server from the BIOS profiles
  matching it, enrolls the secure boot certificates of IncusOS, attaches the
  installation media generated from the given token seed and boots it, and
  watches the server until it has registered itself.

  Not every BMC allows the UEFI key databases to be modified through its Redfish
  API. Use --skip-secure-boot-certificates for such a server, in which case the
  certificates of IncusOS have to be enrolled manually before the deployment is
  triggered.

  The referenced token seed must be public, since the BMC fetches the image
  without authentication, and it should set "force_reboot", so the server
  reboots on its own when the first stage of the installation is done. Use
  --force to accept a token seed without it, in which case the deployment
  relies on the read progress of the installation media alone.

  The BIOS attributes are resolved from the BIOS profiles matching the server.
  Use "server bios-profile" to see what would be applied.

  Use "server deploy-status" to follow the progress and "server deploy-cancel"
  to stop a deployment.
`

	cmd.Flags().StringVar(&c.flagVirtualMediaID, "virtual-media-id", "", `Virtual media device to attach the installation media to, e.g. "system:1". Defaults to the first CD/DVD device of the server, preferring the ones offered by the system`)
	cmd.Flags().StringVar(&c.flagType, "type", "iso", "type of image (iso|raw)")
	cmd.Flags().StringVar(&c.flagArchitecture, "architecture", "x86_64", "CPU architecture for the image (x86_64|aarch64)")
	cmd.Flags().StringVar(&c.flagChannel, "channel", "", "Channel, the most recent update should be taken from to generate the image")
	cmd.Flags().BoolVar(&c.flagForce, "force", false, `Accept a token seed, that does not set "force_reboot"`)
	cmd.Flags().BoolVar(&c.flagSkipSecureBootCertificates, "skip-secure-boot-certificates", false, "Skip the enrollment of the secure boot certificates of IncusOS, they are expected to have been enrolled manually")
	cmd.Flags().BoolVar(&c.flagWait, "wait", false, "Wait for the deployment to complete")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerDeploy) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, 3)
	if exit {
		return err
	}

	return validateImageTypeAndArchitecture(cmd.Flag("type").Value.String(), cmd.Flag("architecture").Value.String())
}

func (c *cmdServerDeploy) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	tokenUUID := args[1]
	seed := args[2]

	err := c.ocClient.DeployServer(cmd.Context(), name, api.ServerDeploymentPost{
		TokenUUID:                  tokenUUID,
		Seed:                       seed,
		Type:                       c.flagType,
		Architecture:               c.flagArchitecture,
		Channel:                    c.flagChannel,
		VirtualMediaID:             c.flagVirtualMediaID,
		Force:                      c.flagForce,
		SkipSecureBootCertificates: c.flagSkipSecureBootCertificates,
	})
	if err != nil {
		return err
	}

	if !c.flagWait {
		return nil
	}

	return c.waitForDeployment(cmd, name)
}

func (c *cmdServerDeploy) waitForDeployment(cmd *cobra.Command, name string) error {
	var reportedUpTo time.Time

	for {
		deployment, err := getServerDeployment(cmd, c.ocClient, name)
		if err != nil {
			return err
		}

		var lines []string

		lines, reportedUpTo = deploymentProgressLines(deployment, reportedUpTo)

		for _, line := range lines {
			fmt.Println(line)
		}

		switch deployment.State {
		case api.ServerDeploymentStateCompleted:
			return nil

		case api.ServerDeploymentStateFailed:
			return fmt.Errorf("Deployment of server %q failed in state %q: %s", name, deployment.FailedState, deployment.LastError)

		case api.ServerDeploymentStateCancelled:
			return fmt.Errorf("Deployment of server %q has been cancelled", name)
		}

		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()

		case <-time.After(deployStatusPollInterval):
		}
	}
}

// deploymentProgressLines renders the states of a deployment, that have not been
// reported up to the given time, and returns the time the last of them has been
// entered.
func deploymentProgressLines(deployment api.ServerDeploymentStatus, reportedUpTo time.Time) ([]string, time.Time) {
	var lines []string

	for _, step := range deployment.History {
		if step.EnteredAt.Before(reportedUpTo) {
			continue
		}

		// The state, that has been reported when it was entered, only needs what
		// it accumulated while the deployment was in it.
		if !step.EnteredAt.Equal(reportedUpTo) {
			lines = append(lines, deploymentStateLine(step.EnteredAt, step.State))

			reportedUpTo = step.EnteredAt
		}

		if step.Retries > 0 {
			lines = append(lines, fmt.Sprintf("  retries: %d", step.Retries))
		}
	}

	if deployment.StateEnteredAt.After(reportedUpTo) {
		lines = append(lines, deploymentStateLine(deployment.StateEnteredAt, deployment.State))

		reportedUpTo = deployment.StateEnteredAt
	}

	return lines, reportedUpTo
}

func deploymentStateLine(enteredAt time.Time, state api.ServerDeploymentState) string {
	return fmt.Sprintf("%s %s", enteredAt.Format(time.RFC3339), state)
}

// Show the status of the deployment of a server.
type cmdServerDeployStatus struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdServerDeployStatus) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "deploy-status <name>"
	cmd.Short = "Show the status of the deployment of a server"
	cmd.Long = `Description:
  Show the status of the deployment of a server, including the state it is
  currently in, the BIOS profiles applied to it and the states it has gone
  through.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerDeployStatus) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	validFormats := []string{"", "json", "yaml"}
	if !slices.Contains(validFormats, c.flagFormat) {
		return fmt.Errorf(`Invalid value for flag "--format": %q`, c.flagFormat)
	}

	return nil
}

func (c *cmdServerDeployStatus) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	deployment, err := getServerDeployment(cmd, c.ocClient, name)
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")

		return enc.Encode(deployment)

	case "yaml":
		enc := yaml.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent(2)

		return enc.Encode(deployment)
	}

	fmt.Printf("State: %s\n", deployment.State)

	if deployment.FailedState != "" {
		fmt.Printf("Failed in: %s\n", deployment.FailedState)
	}

	if deployment.LastError != "" {
		fmt.Printf("Last error: %s\n", deployment.LastError)
	}

	if deployment.Retries > 0 {
		fmt.Printf("Retries: %d\n", deployment.Retries)
	}

	fmt.Printf("Started at: %s\n", deployment.StartedAt.Format(time.RFC3339))
	fmt.Printf("State entered at: %s\n", deployment.StateEnteredAt.Format(time.RFC3339))

	if !deployment.FinishedAt.IsZero() {
		fmt.Printf("Finished at: %s\n", deployment.FinishedAt.Format(time.RFC3339))
	}

	fmt.Printf("Token: %s\n", deployment.Request.TokenUUID)
	fmt.Printf("Seed: %s\n", deployment.Request.Seed)
	fmt.Printf("Virtual media: %s\n", deployment.Request.VirtualMediaID)
	fmt.Printf("Force reboot: %t\n", deployment.ForceReboot)
	fmt.Printf("Skip secure boot certificates: %t\n", deployment.Request.SkipSecureBootCertificates)

	if deployment.MediaURL != "" {
		fmt.Printf("Media URL: %s\n", deployment.MediaURL)
	}

	if deployment.MediaBytesRead >= 0 {
		fmt.Printf("Media read: %s\n", units.GetByteSizeString(deployment.MediaBytesRead, 2))
	}

	fmt.Printf("BIOS profiles: %s\n", strings.Join(deployment.BIOSProfiles, ", "))

	if len(deployment.BIOSAttributes) > 0 {
		renderBIOSAttributes("BIOS attributes", deployment.BIOSAttributes)
	}

	if len(deployment.BIOSDeferredAttributes) > 0 {
		renderBIOSAttributes("BIOS deferred attributes", deployment.BIOSDeferredAttributes)
	}

	if len(deployment.History) > 0 {
		fmt.Printf("History:\n")

		for _, step := range deployment.History {
			fmt.Printf("  %s %s", step.EnteredAt.Format(time.RFC3339), step.State)

			if step.Retries > 0 {
				fmt.Printf(" (retries: %d)", step.Retries)
			}

			if step.Error != "" {
				fmt.Printf(": %s", step.Error)
			}

			fmt.Println()
		}
	}

	return nil
}

// Cancel the deployment of a server.
type cmdServerDeployCancel struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerDeployCancel) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "deploy-cancel <name>"
	cmd.Short = "Cancel the deployment of a server"
	cmd.Long = `Description:
  Cancel the deployment of a server.

  The installation media is ejected and the server is powered off. In contrast,
  a deployment, that failed on its own, is left untouched, so the server can be
  inspected through the BMC.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerDeployCancel) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerDeployCancel) run(cmd *cobra.Command, args []string) error {
	return c.ocClient.CancelServerDeployment(cmd.Context(), args[0])
}

func getServerDeployment(cmd *cobra.Command, ocClient *client.OperationsCenterClient, name string) (api.ServerDeploymentStatus, error) {
	server, err := ocClient.GetServer(cmd.Context(), name)
	if err != nil {
		return api.ServerDeploymentStatus{}, err
	}

	if server.Deployment == nil {
		return api.ServerDeploymentStatus{}, fmt.Errorf("Server %q has never been deployed", name)
	}

	return *server.Deployment, nil
}
