#!/usr/bin/env bash
# Local build and test for the monorepo.
# GitHub Actions are not used: the account plan has Actions locked.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "== build =="
make build

echo "== test =="
make test

echo "check passed"
