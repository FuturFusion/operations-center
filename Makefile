GO ?= go
DETECTED_LIBNBD_VERSION = $(shell dpkg-query --showformat='$${Version}' -W libnbd-dev || echo "0.0.0-libnbd-not-found")

default: build

.PHONY: build
build: operations-center
	$(GO) build -o ./bin/operations-centerd ./cmd/operations-centerd

.PHONY: operations-center
operations-center:
	mkdir -p ./bin/
	CGO_ENABLED=0 GOARCH=amd64 $(GO) build -o ./bin/operations-center.linux.amd64 ./cmd/operations-center
	CGO_ENABLED=0 GOARCH=arm64 $(GO) build -o ./bin/operations-center.linux.arm64 ./cmd/operations-center
	GOOS=darwin GOARCH=amd64 $(GO) build -o ./bin/operations-center.macos.amd64 ./cmd/operations-center
	GOOS=darwin GOARCH=arm64 $(GO) build -o ./bin/operations-center.macos.arm64 ./cmd/operations-center
	GOOS=windows GOARCH=amd64 $(GO) build -o ./bin/operations-center.windows.amd64.exe ./cmd/operations-center
	GOOS=windows GOARCH=arm64 $(GO) build -o ./bin/operations-center.windows.arm64.exe ./cmd/operations-center

.PHONY: build-ui
build-ui:
	$(MAKE) -C ui

.PHONY: build-all-packages
build-all-packages:
	$(GO) mod tidy
	$(GO) build ./...
	$(GO) test -c -o /dev/null ./...

.PHONY: test
test:
	$(GO) test ./... -v

.PHONY: test
test-coverage:
	@rm -rf coverage.out covdata-coverage.out
	@mkdir -p coverage.out
	@echo "================= Running Tests with Coverage ================="
	@go test -cover ./... -coverpkg=github.com/FuturFusion/operations-center/cmd/...,github.com/FuturFusion/operations-center/internal/...,github.com/FuturFusion/operations-center/shared/... -args -test.gocoverdir="$$PWD/coverage.out"
	@echo "================= Coverage Report ================="
	@go tool covdata percent -pkg $$(go tool covdata pkglist -i ./coverage.out | grep -vE '(middleware|mock|version)$$' | paste -sd,) -i=./coverage.out -o covdata-coverage.out | sed 's/%//' | sort -k3,3nr -k1,1 | column -t
	@cat covdata-coverage.out | awk 'BEGIN {cov=0; stat=0;} $$3!="" { cov+=($$3==1?$$2:0); stat+=$$2; } END {printf("Total coverage: %.2f%% of statements\n", (cov/stat)*100);}'

.PHONY: test
test-coverage-func:
	@rm -rf coverage.out covdata-coverage-func.out covdata-coverage-func-filtered.out
	@mkdir -p coverage.out
	@echo "================= Running Tests with Coverage ================="
	@go test -cover ./... -coverpkg=github.com/FuturFusion/operations-center/cmd/...,github.com/FuturFusion/operations-center/internal/...,github.com/FuturFusion/operations-center/shared/... -args -test.gocoverdir="$$PWD/coverage.out"
	@echo "================= Coverage Report ================="
	@go tool covdata textfmt -pkg $$(go tool covdata pkglist -i ./coverage.out | grep -vE '(middleware|mock|version)$$' | paste -sd,) -i=./coverage.out -o covdata-coverage-func.out
	@grep -vE '_gen(_test)?\.go' covdata-coverage-func.out > covdata-coverage-func-filtered.out
	@go tool cover -func covdata-coverage-func-filtered.out | grep -vE '^total' | sed 's/%//' | sort -k3,3nr -k1,1 | column -t
	@cat covdata-coverage-func-filtered.out | awk 'BEGIN {cov=0; stat=0;} $$3!="" { cov+=($$3==1?$$2:0); stat+=$$2; } END {printf("Total coverage: %.2f%% of statements\n", (cov/stat)*100);}'

.PHONY: static-analysis
static-analysis: license-check lint tofu-fmt-check

.PHONY: license-check
license-check:
ifeq ($(shell command -v go-licenses),)
	(cd / ; $(GO) install -v -x github.com/google/go-licenses@latest)
endif
	go-licenses check --disallowed_types=forbidden,unknown,restricted --ignore libguestfs.org/libnbd --ignore github.com/rootless-containers/proto/go-proto ./...

.PHONY: lint
lint:
ifeq ($(shell command -v golangci-lint),)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$($(GO) env GOPATH)/bin
endif
	golangci-lint run ./...
	run-parts $(shell run-parts -V >/dev/null 2>&1 && echo -n "--verbose --exit-on-error --regex '\.sh$$'") scripts/lint

.PHONY: tofu-fmt-check
tofu-fmt-check:
ifeq ($(shell command -v tofu),)
	curl --proto '=https' --tlsv1.2 -fsSL https://get.opentofu.org/install-opentofu.sh | sh -s -- --install-method standalone
endif
	tofu fmt -recursive -check .

.PHONY: vulncheck
vulncheck:
ifeq ($(shell command -v govulncheck),)
	go install golang.org/x/vuln/cmd/govulncheck@latest
endif
	govulncheck ./...

.PHONY: clean
clean:
	rm -rf coverage.out covdata-coverage.out covdata-coverage-func.out covdata-coverage-func-filtered.out
	rm -rf dist/ bin/

.PHONY: release-snapshot
release-snapshot:
ifeq ($(shell command -v goreleaser),)
	echo "Please install goreleaser"
	exit 1
endif
	goreleaser release --snapshot --clean

.PHONY: build-dev-container
build-dev-container:
	docker build -t operations-center-dev ./.devcontainer/

DOCKER_RUN := docker run -i -v .:/home/vscode/src --mount source=operations_center_devcontainer_goroot,target=/go,type=volume --mount source=operations_center_devcontainer_cache,target=/home/vscode/.cache,type=volume --mount source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind -w /home/vscode/src -u 1000:$$(stat -c '%g' /var/run/docker.sock) operations-center-dev

.PHONY: docker-build
docker-build: build-dev-container
	${DOCKER_RUN} make build

.PHONY: docker-build-ui
docker-build-ui: build-dev-container
	${DOCKER_RUN} make build-ui

.PHONY: docker-build-all-packages
docker-build-all-packages: build-dev-container
	${DOCKER_RUN} make build-all-packages

.PHONY: docker-test
docker-test: build-dev-container
	${DOCKER_RUN} make test

.PHONY: docker-static-analysis
docker-static-analysis: build-dev-container
	${DOCKER_RUN} make static-analysis

.PHONY: docker-release-snapshot
docker-release-snapshot: build-dev-container
	${DOCKER_RUN} make release-snapshot

.PHONY: enter-dev-container
enter-dev-container:
	@docker exec -it -w /workspaces/operations-center ${USER}_operations_center_devcontainer /bin/bash

# OpenFGA Syntax Transformer: https://github.com/openfga/syntax-transformer
.PHONY: update-openfga
update-openfga:
	@printf 'package openfga\n\n// Code generated by Makefile; DO NOT EDIT.\n\nvar authModel = `%s`\n' '$(shell $(GO) run github.com/openfga/cli/cmd/fga model transform --file=./internal/authz/openfga/operations-center_model.openfga | jq -c)' > ./internal/authz/openfga/operations-center_model.go

