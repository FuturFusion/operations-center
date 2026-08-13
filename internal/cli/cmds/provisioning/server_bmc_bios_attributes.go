package provisioning

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Interact with a server's BIOS attributes via BMC.
type cmdServerBMCBIOSAttributes struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCBIOSAttributes) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "bios-attributes"
	cmd.Short = "Interact with a server's BIOS attributes via BMC"
	cmd.Long = `Description:
  Interact with a server's BIOS attributes via BMC.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	serverBMCBIOSAttributesListCmd := cmdServerBMCBIOSAttributesList{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCBIOSAttributesListCmd.Command())

	// Show
	serverBMCBIOSAttributesShowCmd := cmdServerBMCBIOSAttributesShow{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCBIOSAttributesShowCmd.Command())

	// Apply BIOS attributes
	serverBMCApplyBIOSAttributesCmd := cmdServerBMCApplyBIOSAttributes{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCApplyBIOSAttributesCmd.Command())

	return cmd
}

// List server's BIOS attributes.
type cmdServerBMCBIOSAttributesList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdServerBMCBIOSAttributesList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list <name>"
	cmd.Short = "List a server's BIOS attributes"
	cmd.Long = `Description:
  List the BIOS attributes known to a server's BMC, along with their type,
  boundaries and current value.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCBIOSAttributesList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdServerBMCBIOSAttributesList) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	attributes, err := c.ocClient.GetServerBMCBIOSAttributes(cmd.Context(), name)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Type", "Current Value"}
	data := [][]string{}

	for _, attribute := range attributes {
		data = append(data, []string{attribute.Name, formatBIOSAttributeType(attribute), fmt.Sprint(attribute.CurrentValue)})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, attributes)
}

func formatBIOSAttributeType(attribute api.BIOSAttribute) string {
	return formatBIOSAttributeTypeAndBounds(attribute.Type, attribute.LowerBound, attribute.UpperBound, attribute.MinLength, attribute.MaxLength)
}

// formatBIOSAttributeTypeAndBounds renders a BIOS attribute's type together
// with its value boundaries (LowerBound/UpperBound for Integer attributes,
// MinLength/MaxLength for String attributes), if any, e.g. "Integer (-20; 20)".
func formatBIOSAttributeTypeAndBounds(typeName string, lowerBound, upperBound, minLength, maxLength *int64) string {
	switch {
	case lowerBound != nil || upperBound != nil:
		return fmt.Sprintf("%s%s", typeName, formatBIOSAttributeBound(lowerBound, upperBound))

	case minLength != nil || maxLength != nil:
		return fmt.Sprintf("%s%s", typeName, formatBIOSAttributeBound(minLength, maxLength))

	default:
		return typeName
	}
}

func formatBIOSAttributeBound(lowerBound *int64, upperBound *int64) string {
	if lowerBound == nil && upperBound == nil {
		return ""
	}

	lower := "-"
	if lowerBound != nil {
		lower = strconv.FormatInt(*lowerBound, 10)
	}

	upper := "-"
	if upperBound != nil {
		upper = strconv.FormatInt(*upperBound, 10)
	}

	return fmt.Sprintf(" (%s; %s)", lower, upper)
}

// Show the acceptable values of a server's BIOS attribute.
type cmdServerBMCBIOSAttributesShow struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdServerBMCBIOSAttributesShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name> <attribute>"
	cmd.Short = "Show the acceptable values of a server's BIOS attribute"
	cmd.Long = `Description:
  Show the current value of a BIOS attribute together with its type.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCBIOSAttributesShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	validFormats := []string{"", "json", "yaml"}
	if !slices.Contains(validFormats, c.flagFormat) {
		return fmt.Errorf(`Invalid value for flag "--format": %q`, c.flagFormat)
	}

	return nil
}

func (c *cmdServerBMCBIOSAttributesShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	attributeName := args[1]

	values, err := c.ocClient.GetServerBMCBIOSAttributeAcceptableValues(cmd.Context(), name, attributeName)
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		err = enc.Encode(values)

	case "yaml":
		enc := yaml.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent(2)
		err = enc.Encode(values)

	default:
		fmt.Printf("Name: %s\n", attributeName)
		fmt.Printf("Type: %s\n", formatBIOSAttributeTypeAndBounds(values.Type, values.LowerBound, values.UpperBound, values.MinLength, values.MaxLength))
		fmt.Printf("Value: %v\n", values.CurrentValue)
		if len(values.AcceptableValues) > 0 {
			fmt.Println("Accepted Values:")
			for _, acceptedValue := range values.AcceptableValues {
				fmt.Printf("  - %s\n", acceptedValue)
			}
		}
	}

	return err
}

// Apply BIOS attributes to a server via BMC.
type cmdServerBMCApplyBIOSAttributes struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCApplyBIOSAttributes) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "apply <name> [attributes.yaml]"
	cmd.Short = "Apply BIOS attributes to a server via BMC"
	cmd.Long = `Description:
  Apply BIOS attributes to a server via BMC

  Applies the given BIOS attributes to the server via BMC. The settings are
  applied on the next reset of the server.

  The attributes are provided as a YAML document with attribute names and
  values at the root level, either from the given file or, if no file is
  given, from stdin.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCApplyBIOSAttributes) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCApplyBIOSAttributes) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	var attributesReader io.Reader = os.Stdin

	if len(args) > 1 {
		attributesFile := args[1]

		f, err := os.Open(attributesFile)
		if err != nil {
			return fmt.Errorf("Failed to read file %q: %w", attributesFile, err)
		}

		defer func() {
			_ = f.Close()
		}()

		attributesReader = f
	}

	body, err := io.ReadAll(attributesReader)
	if err != nil {
		return fmt.Errorf("Failed to read BIOS attributes: %w", err)
	}

	attributes := map[string]any{}

	err = yaml.Unmarshal(body, &attributes)
	if err != nil {
		return fmt.Errorf("Failed to parse BIOS attributes YAML: %w", err)
	}

	err = c.ocClient.ApplyBIOSAttributes(cmd.Context(), name, attributes)
	if err != nil {
		return err
	}

	return nil
}
