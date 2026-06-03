//go:build tools

package tools

import (
	_ "github.com/lxc/incus/v7/cmd/generate-database"
	_ "github.com/openfga/cli/cmd/fga"
	_ "github.com/vektra/mockery/v3"
	_ "golang.org/x/tools/cmd/goimports"
)
