package provisioning

//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-expr generate -p "github.com/FuturFusion/operations-center/internal/provisioning" -p "github.com/FuturFusion/operations-center/shared/api" -p "github.com/lxc/incus-os/incus-osd/api/images" -p "osapi:github.com/lxc/incus-os/incus-osd/api"
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w .
