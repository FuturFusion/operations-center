default: build

# The yarn script used to build the web UI. Set to build-nolint to skip
# eslint, prettier and tsc and only run vite.
UI_BUILD_SCRIPT ?= build

# Directories are part of the sources, so that deletions below src are
# detected as well.
UI_SOURCES := $(shell find src -type f -o -type d) \
              index.html \
              package.json \
              vite.config.ts \
              eslint.config.js \
              $(wildcard tsconfig*.json)

.PHONY: build
build: dist/index.html

dist/index.html: node_modules/.make-install-stamp $(UI_SOURCES)
	yarn $(UI_BUILD_SCRIPT)
	@touch $@

node_modules/.make-install-stamp: package.json yarn.lock
	yarn install
	@touch $@

.PHONY: clean
clean:
	rm -rf dist node_modules/.make-install-stamp
