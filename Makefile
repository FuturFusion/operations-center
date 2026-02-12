GO ?= go
DETECTED_LIBNBD_VERSION = $(shell dpkg-query --showformat='$${Version}' -W libnbd-dev || echo "0.0.0-libnbd-not-found")
SPHINXENV=doc/.sphinx/venv/bin/activate
SPHINXPIPPATH=doc/.sphinx/venv/bin/pip
OPERATIONS_CENTER_E2E_TEST_TMP_DIR=$(shell pwd)/tmp-e2e.out

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

.PHONY: test-coverage
test-coverage:
	@rm -rf coverage.out covdata-coverage.out
	@mkdir -p coverage.out
	@echo "================= Running Tests with Coverage ================="
	@go test -cover ./... -coverpkg=github.com/FuturFusion/operations-center/cmd/...,github.com/FuturFusion/operations-center/internal/...,github.com/FuturFusion/operations-center/shared/... -args -test.gocoverdir="$$PWD/coverage.out"
	@echo "================= Coverage Report ================="
	@go tool covdata percent -pkg $$(go tool covdata pkglist -i ./coverage.out | grep -vE '(middleware|mock|version)$$' | paste -sd,) -i=./coverage.out -o covdata-coverage.out | sed 's/%//' | sort -k3,3nr -k1,1 | column -t
	@cat covdata-coverage.out | awk 'BEGIN {cov=0; stat=0;} $$3!="" { cov+=($$3==1?$$2:0); stat+=$$2; } END {printf("Total coverage: %.2f%% of statements\n", (cov/stat)*100);}'

.PHONY: test-coverage-func
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

.PHONY: update-gomod
update-gomod:
	# Remove gofakeit version pin once we update to Go 1.25+.
	$(GO) get -t -v -u ./... github.com/brianvoe/gofakeit/v7@v7.9.0 github.com/olekukonko/tablewriter@v1.1.0
	$(GO) mod tidy --go=1.24.7
	$(GO) get toolchain@none

.PHONY: update-api
update-api:
	$(GO) install -v -x github.com/go-swagger/go-swagger/cmd/swagger@master
	swagger generate spec -o doc/rest-api.yaml -w ./internal/api -m -x github.com/lxc/incus/v6/shared/api -x github.com/FuturFusion/migration-manager

.PHONY: doc-setup
doc-setup:
	@echo "Setting up documentation build environment"
	python3 -m venv doc/.sphinx/venv
	. $(SPHINXENV) ; pip install --require-virtualenv --upgrade -r doc/.sphinx/requirements.txt --log doc/.sphinx/venv/pip_install.log
	@test ! -f doc/.sphinx/venv/pip_list.txt || \
        mv doc/.sphinx/venv/pip_list.txt doc/.sphinx/venv/pip_list.txt.bak
	$(SPHINXPIPPATH) list --local --format=freeze > doc/.sphinx/venv/pip_list.txt
	rm -Rf doc/html
	rm -Rf doc/.sphinx/.doctrees

.PHONY: doc
doc: doc-setup doc-incremental

.PHONY: doc-incremental
doc-incremental:
	@echo "Build the documentation"
	. $(SPHINXENV) ; sphinx-build -c doc/ -b dirhtml doc/ doc/html/ -d doc/.sphinx/.doctrees -w doc/.sphinx/warnings.txt
	cp doc/rest-api.yaml doc/html/

.PHONY: doc-serve
doc-serve:
	cd doc/html; python3 -m http.server 8001

.PHONY: doc-spellcheck
doc-spellcheck: doc
	. $(SPHINXENV) ; python3 -m pyspelling -c doc/.sphinx/spellingcheck.yaml

.PHONY: doc-linkcheck
doc-linkcheck: doc-setup
	. $(SPHINXENV) ; LOCAL_SPHINX_BUILD=True sphinx-build -c doc/ -b linkcheck doc/ doc/html/ -d doc/.sphinx/.doctrees

.PHONY: doc-lint
doc-lint:
	doc/.sphinx/.markdownlint/doc-lint.sh

.PHONY: e2e-test
e2e-test: build
	mkdir -p $(OPERATIONS_CENTER_E2E_TEST_TMP_DIR)
	OPERATIONS_CENTER_E2E_TEST=1 OPERATIONS_CENTER_E2E_TEST_TMP_DIR=$(OPERATIONS_CENTER_E2E_TEST_TMP_DIR) go test ./e2e_tests/ -v -timeout 60m -count 1 | tee $$OPERATIONS_CENTER_E2E_TEST_TMP_DIR/e2e_tests_$$(date +%F-%H-%M-%S).log

.PHONY: clean-e2e-test
clean-e2e-test:
	rm -rf $(OPERATIONS_CENTER_E2E_TEST_TMP_DIR)
	rm -rf $$HOME/.config/operations-center/
	incus remote remove incus-os-cluster || true
	incus remote remove incus-os-cluster-after-factory-reset-1 || true
	incus remote remove incus-os-cluster-after-factory-reset-2 || true
	incus remove --force OperationsCenter || true
	incus remove --force IncusOS01 || true
	incus remove --force IncusOS02 || true
	incus remove --force IncusOS03 || true
	incus storage volume delete default IncusOS_OperationsCenter.iso || true
	for i in $$(incus storage volume list default -f json | jq -r '.[] | select(.name | test("IncusOS-.*")) | .name'); do \
		incus storage volume delete default $$i || true; \
	done
