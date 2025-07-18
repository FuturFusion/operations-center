package render

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"
)

// Table list format.
const (
	TableFormatCSV     = "csv"
	TableFormatJSON    = "json"
	TableFormatTable   = "table"
	TableFormatYAML    = "yaml"
	TableFormatCompact = "compact"
)

const (
	// TableOptionNoHeader hides the table header when possible.
	TableOptionNoHeader = "noheader"

	// TableOptionHeader adds header to csv.
	TableOptionHeader = "header"
)

// Table renders tabular data in various formats.
func Table(w io.Writer, format string, header []string, data [][]string, raw any) error {
	if w == nil {
		return fmt.Errorf("Unable to render to nil writer")
	}

	fields := strings.SplitN(format, ",", 2)
	format = fields[0]

	var options []string
	if len(fields) == 2 {
		options = strings.Split(fields[1], ",")

		if slices.Contains(options, TableOptionNoHeader) {
			header = nil
		}
	}

	switch format {
	case TableFormatTable:
		table := getBaseTable(w, header, data)
		table.SetRowLine(true)
		table.Render()
	case TableFormatCompact:
		table := getBaseTable(w, header, data)
		table.SetColumnSeparator("")
		table.SetHeaderLine(false)
		table.SetBorder(false)
		table.Render()
	case TableFormatCSV:
		w := csv.NewWriter(w)
		if slices.Contains(options, TableOptionHeader) {
			err := w.Write(header)
			if err != nil {
				return err
			}
		}

		err := w.WriteAll(data)
		if err != nil {
			return err
		}

	case TableFormatJSON:
		enc := json.NewEncoder(w)

		err := enc.Encode(raw)
		if err != nil {
			return err
		}

	case TableFormatYAML:
		out, err := yaml.Marshal(raw)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(w, "%s", out)
	default:
		// TODO: should this go to stderr?
		return fmt.Errorf("Invalid format %q", format)
	}

	return nil
}

func getBaseTable(w io.Writer, header []string, data [][]string) *tablewriter.Table {
	table := tablewriter.NewWriter(w)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeader(header)
	table.AppendBulk(data)
	return table
}

// Column represents a single column in a table.
type Column struct {
	Header string

	// DataFunc is a method to retrieve data for this column. The argument to this function will be an element of the
	// "data" slice that is passed into RenderSlice.
	DataFunc func(any) (string, error)
}
