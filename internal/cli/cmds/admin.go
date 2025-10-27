package cmds

import (
	"net/http"
	"net/url"

	"github.com/lxc/incus-os/incus-osd/cli"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/client"
)

type CmdAdmin struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdAdmin) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "admin"
	cmd.Short = "Manage IncusOS"
	cmd.Long = `Description:
  Manage IncusOS
`

	// os
	adminOSCmd := cmdAdminOS{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(adminOSCmd.Command())

	return cmd
}

type cmdAdminOS struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdAdminOS) Command() *cobra.Command {
	args := &cli.Args{
		SupportsTarget:    false,
		SupportsRemote:    false,
		DefaultListFormat: "table",
		DoHTTP: func(_ string, req *http.Request) (*http.Response, error) {
			var err error

			req.URL, err = url.Parse(c.ocClient.GetBaseAddr() + req.URL.String())
			if err != nil {
				return nil, err
			}

			return c.ocClient.DoHTTP(req)
		},
	}

	return cli.NewCommand(args)
}
