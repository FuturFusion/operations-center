package tools

//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory

//go:generate go run github.com/vektra/mockery/v3 --log-level WARN
//go:generate go run github.com/vektra/mockery/v3 --log-level WARN --config .mockery-slog.yaml
//go:generate go run github.com/vektra/mockery/v3 --log-level WARN --config .mockery-prometheus.yaml
