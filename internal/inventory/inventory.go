package inventory

import "github.com/google/uuid"

//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-expr generate -p "github.com/FuturFusion/operations-center/internal/inventory" -p "github.com/lxc/incus/v6/shared/api"
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w .

var InventorySpaceUUID = uuid.MustParse(`00000000-0000-0000-0000-000000000000`)
