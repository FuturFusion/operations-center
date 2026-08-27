package provisioning

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Show the BIOS profiles resolved for a server.
type cmdServerBIOSProfile struct {
	ocClient *client.OperationsCenterClient

	flagFormat   string
	flagValidate bool
}

func (c *cmdServerBIOSProfile) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "bios-profile <name>"
	cmd.Short = "Show the BIOS profiles resolved for a server"
	cmd.Long = `Description:
  Show the BIOS attributes and the secure boot configuration, that would be
  applied to a server before IncusOS is installed on it.

  They are accumulated from all the BIOS profiles matching the data reported by
  the BMC of the server, processed by priority ascending, so a profile with a
  higher priority extends or overwrites what the profiles with a lower priority
  have contributed.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)
	cmd.Flags().BoolVar(&c.flagValidate, "validate", false, "Validate the resolved attributes against the BIOS attribute registry of the server's BMC")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBIOSProfile) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
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

func (c *cmdServerBIOSProfile) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	resolution, err := c.ocClient.GetServerBIOSProfile(cmd.Context(), name, c.flagValidate)
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")

		return enc.Encode(resolution)

	case "yaml":
		enc := yaml.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent(2)

		return enc.Encode(resolution)

	default:
		fmt.Printf("Profiles: %s\n", strings.Join(resolution.Profiles, ", "))

		fmt.Printf("Attributes:\n")

		for _, attribute := range slices.Sorted(maps.Keys(resolution.Attributes)) {
			fmt.Printf("  %s: %v\n", attribute, resolution.Attributes[attribute])
		}

		return renderSecureBoot(resolution.SecureBoot)
	}
}

func renderSecureBoot(secureBoot api.BIOSSecureBoot) error {
	databases := []struct {
		name    string
		entries api.BIOSSecureBootDatabase
	}{
		{name: "db", entries: secureBoot.DB},
		{name: "dbx", entries: secureBoot.DBX},
		{name: "KEK", entries: secureBoot.KEK},
	}

	for _, database := range databases {
		if len(database.entries.Certificates) == 0 && len(database.entries.Signatures) == 0 {
			continue
		}

		entriesYAML, err := yaml.Marshal(database.entries)
		if err != nil {
			return err
		}

		fmt.Printf("Secure Boot %s:\n%s\n", database.name, render.Indent(2, strings.TrimSpace(string(entriesYAML))))
	}

	return nil
}
