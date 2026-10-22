package tools

//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory

//go:generate go run github.com/vektra/mockery/v3 --log-level WARN
//go:generate go run github.com/vektra/mockery/v3 --log-level WARN --config .mockery-slog.yaml
//go:generate go run github.com/vektra/mockery/v3 --log-level WARN --config .mockery-prometheus.yaml
//go:generate go run github.com/vektra/mockery/v3 --log-level WARN --config .mockery-error-wrapper.yaml

//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-components
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-deployment-diagram
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-filtered-architecture-def
//go:generate go tool go-arch-lint graph --arch-file .go-arch-lint-overview_gen.yml --out doc/images/architecture-daemon.svg --focus daemon
//go:generate go tool go-arch-lint graph --arch-file .go-arch-lint-overview_gen.yml --out doc/images/architecture-cli.svg --focus cli
