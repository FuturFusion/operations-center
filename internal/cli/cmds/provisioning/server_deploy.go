package provisioning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	flagSecureBootEnrollmentMedia  bool
	flagBIOSProfiles               []string
	flagSecureBootCertificates     []string
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
  API. Use --secure-boot-enrollment-media for such a server: Operations Center
  then clears its key databases through the BMC, which puts it into the secure
  boot setup mode, and boots a generated enrollment media, that enrolls the
  certificates of IncusOS. Alternatively the secure boot certificates can also
  be enrolled manually. Use --skip-secure-boot-certificates in this case or if
  the server does not support secure boot at all.

  The referenced token seed must be public, since the BMC fetches the image
  without authentication, and it should set "force_reboot", so the server
  reboots on its own when the first stage of the installation is done. Use
  --force to accept a token seed without it, in which case the deployment
  relies on the read progress of the installation media alone.

  The BIOS attributes are resolved from the BIOS profiles matching the server.
  Use "server bios-profile" to see what would be applied.

  Use --bios-profiles to resolve the BIOS configuration, which is the BIOS
  attributes, the secure boot allow lists and the deployment settings, from the
  BIOS profiles of a file instead. The file holds BIOS profiles in the format of the ones shipped with
  Operations Center, they are matched against the server and accumulated by
  priority the same way. Certificates, that such a profile keeps in a secure
  boot database and that Operations Center does not know, have to be provided
  with --secure-boot-certificate for the enrollment media to enroll them.

  Use "server deploy-status" to follow the progress and "server deploy-cancel"
  to stop a deployment.
`

	cmd.Flags().StringVar(&c.flagVirtualMediaID, "virtual-media-id", "", `Virtual media device to attach the installation media to, e.g. "system:1". Defaults to the first device of the server taking the requested image type, preferring the ones offered by the system`)
	cmd.Flags().StringVar(&c.flagType, "type", "iso", "type of image (iso|raw)")
	cmd.Flags().StringVar(&c.flagArchitecture, "architecture", "", "CPU architecture for the images (x86_64|aarch64), taken from the BMC of the server, when not given")
	cmd.Flags().StringVar(&c.flagChannel, "channel", "", "Channel, the most recent update should be taken from to generate the image")
	cmd.Flags().BoolVar(&c.flagForce, "force", false, `Accept a token seed, that does not set "force_reboot"`)
	cmd.Flags().BoolVar(&c.flagSkipSecureBootCertificates, "skip-secure-boot-certificates", false, "Skip the enrollment of the secure boot certificates of IncusOS, they are expected to have been enrolled manually")
	cmd.Flags().BoolVar(&c.flagSecureBootEnrollmentMedia, "secure-boot-enrollment-media", false, "Enroll the secure boot certificates of IncusOS by booting a generated enrollment media instead of through the Redfish API")
	cmd.Flags().StringArrayVar(&c.flagBIOSProfiles, "bios-profiles", nil, "YAML file with the BIOS profiles to resolve the BIOS configuration from instead of the ones shipped with Operations Center (can be given multiple times)")
	cmd.Flags().StringArrayVar(&c.flagSecureBootCertificates, "secure-boot-certificate", nil, "PEM file with an additional secure boot certificate kept by the provided BIOS profiles, requires --bios-profiles and --secure-boot-enrollment-media (can be given multiple times)")
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

	biosProfiles, err := readBIOSProfiles(c.flagBIOSProfiles)
	if err != nil {
		return err
	}

	secureBootCertificates := make([]string, 0, len(c.flagSecureBootCertificates))
	for _, filename := range c.flagSecureBootCertificates {
		certificate, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("Failed to read secure boot certificate: %w", err)
		}

		secureBootCertificates = append(secureBootCertificates, string(certificate))
	}

	err = c.ocClient.DeployServer(cmd.Context(), name, api.ServerDeploymentPost{
		TokenUUID:                  tokenUUID,
		Seed:                       seed,
		Type:                       c.flagType,
		Architecture:               c.flagArchitecture,
		Channel:                    c.flagChannel,
		VirtualMediaID:             c.flagVirtualMediaID,
		Force:                      c.flagForce,
		SkipSecureBootCertificates: c.flagSkipSecureBootCertificates,
		SecureBootEnrollmentMedia:  c.flagSecureBootEnrollmentMedia,
		BIOSProfiles:               biosProfiles,
		SecureBootCertificates:     secureBootCertificates,
	})
	if err != nil {
		return err
	}

	if !c.flagWait {
		return nil
	}

	deployment, err := waitForDeployment(cmd, c.ocClient, name, cmd.OutOrStdout())
	if err != nil {
		return err
	}

	return deploymentResultError(name, deployment)
}

// readBIOSProfiles reads the BIOS profiles from YAML files.
func readBIOSProfiles(filenames []string) ([]api.BIOSProfile, error) {
	var profiles []api.BIOSProfile

	for _, filename := range filenames {
		content, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("Failed to read BIOS profiles: %w", err)
		}

		fileProfiles, err := parseBIOSProfiles(content)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse BIOS profiles %q: %w", filename, err)
		}

		if len(fileProfiles) == 0 {
			return nil, fmt.Errorf("BIOS profiles file %q does not hold any profile", filename)
		}

		profiles = append(profiles, fileProfiles...)
	}

	return profiles, nil
}

// parseBIOSProfiles parses a list of BIOS profiles, like the files of the catalog hold one, or a single one.
func parseBIOSProfiles(content []byte) ([]api.BIOSProfile, error) {
	var document yaml.Node

	err := yaml.Unmarshal(content, &document)
	if err != nil {
		return nil, err
	}

	if len(document.Content) == 0 {
		return nil, nil
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	if document.Content[0].Kind == yaml.MappingNode {
		profile := api.BIOSProfile{}

		err = decoder.Decode(&profile)
		if err != nil {
			return nil, err
		}

		return []api.BIOSProfile{profile}, nil
	}

	profiles := []api.BIOSProfile{}

	err = decoder.Decode(&profiles)
	if err != nil {
		return nil, err
	}

	return profiles, nil
}

// waitForDeployment polls the deployment of the given server until it reaches a
// terminal state and returns the deployment in that state. Every state, that is
// entered, is reported to the given writer, a nil writer silencing the report.
func waitForDeployment(cmd *cobra.Command, ocClient *client.OperationsCenterClient, name string, progress io.Writer) (api.ServerDeploymentStatus, error) {
	var reportedUpTo time.Time

	for {
		deployment, err := getServerDeployment(cmd, ocClient, name)
		if err != nil {
			return api.ServerDeploymentStatus{}, err
		}

		var lines []string

		lines, reportedUpTo = deploymentProgressLines(deployment, reportedUpTo)

		if progress != nil {
			for _, line := range lines {
				_, _ = fmt.Fprintln(progress, line)
			}
		}

		if deployment.State.IsTerminal() {
			return deployment, nil
		}

		select {
		case <-cmd.Context().Done():
			return api.ServerDeploymentStatus{}, cmd.Context().Err()

		case <-time.After(deployStatusPollInterval):
		}
	}
}

// deploymentResultError reports the outcome of a finished deployment as an error,
// a deployment, that completed, resulting in no error.
func deploymentResultError(name string, deployment api.ServerDeploymentStatus) error {
	switch deployment.State {
	case api.ServerDeploymentStateFailed:
		return fmt.Errorf("Deployment of server %q failed in state %q: %s", name, deployment.FailedState, deployment.LastError)

	case api.ServerDeploymentStateCancelled:
		return fmt.Errorf("Deployment of server %q has been cancelled", name)
	}

	return nil
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
	flagWait   bool
}

func (c *cmdServerDeployStatus) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "deploy-status <name>"
	cmd.Short = "Show the status of the deployment of a server"
	cmd.Long = `Description:
  Show the status of the deployment of a server, including the state it is
  currently in, the BIOS profiles applied to it and the states it has gone
  through.

  Use --wait to follow a running deployment until it is done. Combined
  with --format, nothing is reported while waiting and the status is shown once
  the deployment has finished.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)
	cmd.Flags().BoolVar(&c.flagWait, "wait", false, "Wait for the deployment to complete")

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

	var (
		deployment api.ServerDeploymentStatus
		err        error
	)

	if c.flagWait {
		var progress io.Writer
		if c.flagFormat == "" {
			progress = cmd.OutOrStdout()
		}

		deployment, err = waitForDeployment(cmd, c.ocClient, name, progress)
	} else {
		deployment, err = getServerDeployment(cmd, c.ocClient, name)
	}

	if err != nil {
		return err
	}

	var resultErr error
	if c.flagWait {
		resultErr = deploymentResultError(name, deployment)
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")

		err = enc.Encode(deployment)
		if err != nil {
			return err
		}

		return resultErr

	case "yaml":
		enc := yaml.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent(2)

		err = enc.Encode(deployment)
		if err != nil {
			return err
		}

		return resultErr
	}

	if c.flagWait {
		return resultErr
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
	fmt.Printf("Secure boot enrollment media: %t\n", deployment.Request.SecureBootEnrollmentMedia)

	if deployment.MediaURL != "" {
		fmt.Printf("Media URL: %s\n", deployment.MediaURL)
	}

	if deployment.SecureBootMediaURL != "" {
		fmt.Printf("Secure boot media URL: %s\n", deployment.SecureBootMediaURL)
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

	err = renderDeploymentSettings("Deployment settings", deployment.DeploymentSettings)
	if err != nil {
		return err
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

	flagSkipCleanup bool
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

	cmd.Flags().BoolVar(&c.flagSkipCleanup, "skip-cleanup", false, "Stop the deployment without ejecting the installation media and without powering the server off")

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
	return c.ocClient.CancelServerDeployment(cmd.Context(), args[0], c.flagSkipCleanup)
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
