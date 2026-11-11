.PHONY: help check build test cli-build web-test desktop-deps desktop-test desktop-build \
	rust-test js-test demo-check sync-check import-status

help:
	@printf '%s\n' \
	  'check           Build and test locally (./scripts/check.sh)' \
	  'build           Build the CLI and the desktop web bundle' \
	  'test            Run in-tree tests and satellite tests when those trees exist' \
	  'cli-build       go build -o dist/devpod .' \
	  'web-test        go test ./cmd/web/...' \
	  'desktop-test    Yarn install and Vitest in desktop/' \
	  'desktop-build   Yarn install and yarn build:web' \
	  'rust-test       cargo test --workspace when Rust satellites are imported' \
	  'js-test         npm test for devpod-pro-ui and devpod-e2e-tests when imported' \
	  'demo-check      Parse examples/devpod-demo devcontainer.json' \
	  'sync-check      Check Cargo members and npm workspaces against the manifest' \
	  'import-status   Print monorepo.manifest.json import status'

check:
	./scripts/check.sh

build: cli-build desktop-build

test: web-test desktop-test rust-test js-test demo-check

cli-build:
	mkdir -p dist
	go build -o dist/devpod .

web-test:
	go test ./cmd/web/...

desktop-deps:
	yarn --cwd desktop install --frozen-lockfile

desktop-test: desktop-deps
	yarn --cwd desktop test

desktop-build: desktop-deps
	yarn --cwd desktop build:web

sync-check:
	python3 hack/sync-workspaces.py --check

rust-test: sync-check
	@members=$$(python3 hack/sync-workspaces.py --list rust); \
	if [ -z "$$members" ]; then \
	  echo "rust: devpod-api, devpod-enterprise, and devpod-ssh-gateway are not imported; skipping cargo test"; \
	else \
	  cargo test --workspace --locked; \
	fi

js-test: sync-check
	@members=$$(python3 hack/sync-workspaces.py --list js); \
	if [ -z "$$members" ]; then \
	  echo "js: devpod-pro-ui and devpod-e2e-tests are not imported; skipping npm test"; \
	else \
	  npm install; \
	  npm test --workspaces --if-present; \
	fi

demo-check:
	python3 -c 'import json; p="examples/devpod-demo/.devcontainer/devcontainer.json"; json.load(open(p)); print("demo devcontainer json ok", p)'

import-status:
	./hack/import-satellites.sh --status
