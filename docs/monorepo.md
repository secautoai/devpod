# DevPod monorepo, phase 1

Phase 1 lays out one repository for the DevPod stack and imports every satellite this environment can read. Nothing here merges to `main`, and the satellite GitHub repositories are left untouched.

## Base

The root tree is [secautoai/devpod](https://github.com/secautoai/devpod) at `claude/add-web-ui-W5nzb` (`464810e526a8c3940630ed3348bc5f9761ee863e`). That branch is 17 commits ahead of `main` (`b358e1b0697bc441de54fed6a5a032489ce1ae57`) and 0 behind. It is the owner's web UI work: `devpod web`, multi-user auth, and the desktop client paths that call it.

GitHub records this repo as a fork of [skevetter/devpod](https://github.com/skevetter/devpod), whose source is [loft-sh/devpod](https://github.com/loft-sh/devpod). The Go module path remains `github.com/skevetter/devpod`. The root license remains MPL-2.0 (`LICENSE`).

## Layout

The CLI stays at the repository root. Moving `cmd/`, `pkg/`, `desktop/`, and `go.mod` into a subdirectory would rewrite every upstream path and make a later diff against a local monorepo harder. Satellites use the GitHub repository name as the directory name so a subtree split round-trips.

| Path | Source | Build |
| --- | --- | --- |
| `/` | this fork, web-UI branch | `go build`, Yarn in `desktop/` |
| `devpod-api/` | secautoai/devpod-api | Cargo workspace member |
| `devpod-enterprise/` | secautoai/devpod-enterprise | Cargo workspace member |
| `devpod-ssh-gateway/` | secautoai/devpod-ssh-gateway | Cargo workspace member |
| `devpod-pro-ui/` | secautoai/devpod-pro-ui | npm workspace |
| `devpod-e2e-tests/` | secautoai/devpod-e2e-tests | npm workspace |
| `examples/devpod-demo/` | secautoai/devpod-demo `main` | devcontainer fixture |
| `e2e/` | already in this fork | Ginkgo; separate from Playwright |
| `desktop/src-tauri/` | already in this fork | Tauri crate, outside the Cargo workspace |

`desktop/` stays on Yarn 1 and `desktop/yarn.lock`. It is not an npm workspace, because hoisting it to the root would change the existing desktop install. `hack/sync-workspaces.py` fills `Cargo.toml` `members` and the root `package.json` `workspaces` only from satellites whose files are actually present.

## devpod-backend

`devpod-backend` is a DevPod CLI fork under MPL-2.0. This repository is already that CLI, including the web UI branch. Importing it under a second prefix would create two `go.mod` trees, two `cmd/` trees, and two desktop apps.

It is not imported. `hack/import-satellites.sh` never runs `git subtree add` for it. When the remote is readable the script clones it in a temporary bare repo, prints commits reachable from its `HEAD` that are absent from this branch, and stops. Those commits belong on the repository root (cherry-pick or merge), still as one CLI. A `devpod-backend/` directory is the wrong reconciliation, including against the local monorepo described below.

This environment could not fetch the remote, so that commit list is still empty. See import status.

## Import status

Counted on 2026-11-09. "Commits" is the number of commits reachable from the imported tip (`git rev-list --count <commit>`), which for a subtree is the satellite history that was brought across. Subtree merges are additional commits on this branch and are not double-counted.

| Satellite | Status | Commits | Imported tip | License |
| --- | --- | --- | --- | --- |
| devpod-backend | not imported (duplicate CLI role) | unobserved | — | MPL-2.0, reported by the owner, file not seen |
| devpod-api | not imported | unobserved | — | unobserved; no license assigned |
| devpod-enterprise | not imported | unobserved | — | unobserved; no license assigned |
| devpod-ssh-gateway | not imported | unobserved | — | unobserved; no license assigned |
| devpod-pro-ui | not imported | unobserved | — | unobserved; no license assigned |
| devpod-e2e-tests | not imported | unobserved | — | unobserved; no license assigned |
| devpod-demo | imported at `examples/devpod-demo` | 1 | `ffc90d6ffd332c370f1716816310b2c60fc01520` | no license file; none chosen |

The demo import is a `git subtree add` with no squash. The subtree merge records:

- `git-subtree-dir: examples/devpod-demo`
- `git-subtree-split: ffc90d6ffd332c370f1716816310b2c60fc01520`

`devpod-demo` also has branch `claude/compiler-principles-rust-tutorial-t5dmsc` at `0088b2b0201efea402b7e427b7df33a855d37d1f` (a compiler tutorial). That branch is not the devcontainer fixture, so it stays only on the satellite.

Private satellites returned HTTP 404 to the GitHub App credential installed for this run. `GET /installation/repositories` lists only `secautoai/devpod`. Unauthenticated `git ls-remote` returns "repository not found", which is what GitHub returns for private repositories this token cannot see. Their history is therefore not in this branch. `hack/import-satellites.sh` is the import for when a credential can read them: it resolves `HEAD` with `git ls-remote --symref` (required for `devpod-ssh-gateway`, whose default branch is not assumed to be `main`), then `git subtree add` with full history. It only fetches. It does not push.

## License map

MPL-2.0 portions stay MPL-2.0. This migration does not pick a license for unlicensed owner code and does not invent one for repositories whose `LICENSE` file was not readable.

| Tree | License | Attribution |
| --- | --- | --- |
| Repository root (CLI, `desktop/`, `e2e/`, `providers/`, `docs/`) | MPL-2.0 | Root `LICENSE`. Upstream DevPod copyright stays in place. Module `github.com/skevetter/devpod`. |
| devpod-backend | MPL-2.0 (owner report) | Not copied, so there is no second MPL tree to keep. Any later root-level commits from that fork keep their MPL headers. |
| devpod-api, devpod-enterprise, devpod-ssh-gateway, devpod-pro-ui, devpod-e2e-tests | Not observed | On import, each repo's `LICENSE`, `NOTICE`, and headers stay in its prefix. No license will be added if the tree has none. |
| `examples/devpod-demo/` | None in the source | Files match the satellite tip. No `LICENSE` was added. |

## Reconciling local commit `71f0995c`

A local `devpod-monorepo` at commit `71f0995c` was reported and has no remote. It was not on disk here. A public GitHub commit whose hash starts with `71f0995c` belongs to an unrelated repository (`webmturn/sk_note`) and is not this monorepo.

Assumptions, so a later diff has a fixed target:

1. Compare that checkout to this branch by path. The CLI files line up at the repository root, not under `cli/` or `devpod-backend/`.
2. Satellite prefixes match the table above. If the local checkout used the same prefixes, `git diff` is a content diff. If it used different prefixes, reconcile with `git mv` onto these names, then subtree-split to confirm the satellite tip.
3. Future imports stay unsquashed, so `git-subtree-split` still names the satellite commit.
4. `monorepo.manifest.json` is the machine-readable copy of this contract. `make import-status` prints it.
5. `devpod-backend` in the local checkout, if present as a full CLI tree, is the duplicate described above. Keep one CLI at the root. Unique commits from the other history are cherry-picked onto that root.
6. Side-branch `0088b2b` of `devpod-demo` is intentionally absent. Copy it only if that tutorial is meant to live in this repo.
7. No satellite remote is archived or rewritten by this work.

Check that the imported demo tree is still the satellite tip:

```bash
test "$(git rev-parse HEAD:examples/devpod-demo)" = "$(git rev-parse ffc90d6ffd332c370f1716816310b2c60fc01520^{tree})"
```

`git subtree split --prefix=examples/devpod-demo` is the usual way to re-extract a prefix, but the subtree helper in Git walks history with a shell function and hits a recursion limit on this CLI's history. The original demo commit is already a parent of the subtree merge (`git-subtree-split: ffc90d6ffd332c370f1716816310b2c60fc01520`), so that commit is the extract until a later commit changes files under the prefix.

## Cross-repo dependencies

`devpod-pro-ui` documents its coupling in `TODO-cross-repo-dependencies.md`. That file was not readable, so those items are not resolved. No client package was invented in its place.

What this tree already agrees on, and the one mismatch that was fixed:

- `desktop/src/client/authClient.ts`, `desktop/vite-dev-api.ts`, and `cmd/web/web.go` all use `POST /api/login` and `GET /api/me`.
- `docs/pages/developing/web-ui.mdx` had diagrammed `/api/auth/login` and `/api/auth/me`. The diagram now matches the server and the client.
- `desktop/src/lib/platform.ts` derives `WEB_SERVER_URL` from `window.location.origin` in web mode (port 8090 is the `devpod web` default).

The in-tree Go server routes are the contract the desktop web UI calls today:

| Method | Path |
| --- | --- |
| GET | `/api/health`, `/api/version`, `/api/platform`, `/api/branding` |
| GET | `/releases` |
| POST | `/api/login`, `/api/command`, `/api/signal`, `/api/user/password` |
| GET | `/api/me`, `/api/providers`, `/api/workspaces`, `/api/logs` |
| WebSocket | `/api/command/stream` |
| admin | `/api/admin/users`, `/api/admin/users/`, `/api/admin/workspaces`, `/api/admin/logs`, `/api/admin/branding` |

A separate note in the owner's Notion page "The harness Dev" shows a Rust `devpod-api` used as `POST /api/workspaces/:id/up` (SSE) and `GET /api/workspaces/:id/ide-url`. Those paths are a different surface from `devpod web`. They were not implemented here, because the Rust source was not available to match.

`cmd/web` issues its own HS256 JWTs (`github.com/golang-jwt/jwt/v5`) when `devpod web` runs with `--auth`. `devpod-enterprise` is also described as Rust JWT. Those two implementations were not compared.

## Build and test

GitHub Actions are not part of this phase. The account is on the free plan and Actions are locked, so no workflow file was added.

Run the same build and test locally:

```bash
./scripts/check.sh
# equivalent: make check
```

`scripts/check.sh` runs `make build` and then `make test`:

| Target | What it does |
| --- | --- |
| `make build` | `go build -o dist/devpod .` and `yarn build:web` in `desktop/` |
| `make web-test` | `go test ./cmd/web/...` |
| `make desktop-test` | Yarn install (frozen lockfile) and Vitest |
| `make rust-test` | `cargo test --workspace` when Rust satellites are imported; otherwise a skip line |
| `make js-test` | `npm test` for `devpod-pro-ui` and `devpod-e2e-tests` when imported; otherwise a skip line |
| `make demo-check` | Parse `examples/devpod-demo/.devcontainer/devcontainer.json` |

`make rust-test` and `make js-test` fail if `Cargo.toml` or `package.json` drift from the directories on disk (`make sync-check`).

## Parity still open

- **API.** `devpod-api` is not in the tree. The Go `devpod web` routes above are not the workspace up/ide-url API attributed to the Rust service. No response-shape diff was run.
- **UI.** `devpod-pro-ui` and `TODO-cross-repo-dependencies.md` are not in the tree. The desktop web UI in this repo is covered by its Vitest tests and by `go test ./cmd/web/...`. A browser pass against `devpod-pro-ui` was not possible.
- **SSH gateway.** `devpod-ssh-gateway` is not in the tree. No `russh` build or handshake test was run.
- **E2E.** Playwright (`devpod-e2e-tests`) is not in the tree. The Ginkgo suite in `e2e/` needs a built CLI and a provider; it was not run in this phase.
- **CLI delta.** Unique `devpod-backend` commits, if any, are not identified until that remote can be fetched.
- **Local monorepo.** Commit `71f0995c` was not available to diff.
