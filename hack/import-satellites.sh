#!/usr/bin/env bash
# Import DevPod satellites into the prefixes in monorepo.manifest.json.
# Uses git subtree so each satellite's commits stay in this repository.
# Fetches only. Does not push, archive, or otherwise modify the satellites.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

usage() {
  cat <<'EOF'
Usage: hack/import-satellites.sh [--status | --dry-run | --import]

  --status   Print the manifest. Does not use the network.
  --dry-run  Resolve each satellite's default branch. Does not write to the tree.
  --import   Subtree-add satellites that are not already present (default).

devpod-backend is never added as a directory. When that remote is readable,
commits that are absent from HEAD are printed for a root-level review.
EOF
}

mode=import
case "${1:-}" in
  ""|--import) mode=import ;;
  --status) mode=status ;;
  --dry-run) mode=dry-run ;;
  -h|--help) usage; exit 0 ;;
  *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
esac

python3 - <<'PY'
import json
m=json.load(open("monorepo.manifest.json"))
print(f"cli {m['cli']['commit']} ({m['cli']['branch']})")
print(f"local monorepo reported commit {m['localMonorepo']['reportedCommit']} remote={m['localMonorepo']['remote']}")
for s in m["satellites"]:
    prefix=s.get("prefix") or "-"
    commit=s.get("commit") or "-"
    count=s.get("commitCount")
    count_s="-" if count is None else str(count)
    print(f"{s['status']:13} {s['name']:22} {prefix:24} commits={count_s:4} {commit}")
PY

if [[ "$mode" == status ]]; then
  exit 0
fi

if [[ "$mode" == import ]]; then
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "working tree is dirty; commit or stash before importing" >&2
    exit 1
  fi
fi

failures=0

resolve_branch() {
  local url=$1
  local wanted=$2
  if [[ -n "$wanted" && "$wanted" != "null" ]]; then
    printf '%s' "$wanted"
    return 0
  fi
  local symref
  if ! symref=$(git ls-remote --symref "$url" HEAD | awk '/^ref:/ {sub("refs/heads/","",$2); print $2; exit}'); then
    return 1
  fi
  if [[ -z "$symref" ]]; then
    return 1
  fi
  printf '%s' "$symref"
}

note_failure() {
  echo "$1" >&2
  failures=$((failures + 1))
}

inspect_backend() {
  local url=$1
  if [[ "$mode" == dry-run ]]; then
    echo "devpod-backend: resolve HEAD only; a directory is never added"
    if ! git ls-remote --symref "$url" HEAD >/dev/null; then
      note_failure "devpod-backend: unreachable"
    fi
    return 0
  fi
  local tmp
  tmp=$(mktemp -d)
  echo "devpod-backend: fetching $url to compare history (no prefix will be added)"
  if ! git clone --bare --quiet "$url" "$tmp/repo.git" 2>"$tmp/err"; then
    echo "devpod-backend: remote is not readable. No second CLI tree was added."
    sed 's/^/  /' "$tmp/err" >&2 || true
    rm -rf "$tmp"
    note_failure "devpod-backend: unreachable"
    return 0
  fi
  local head mod unique
  head=$(git -C "$tmp/repo.git" rev-parse HEAD)
  mod=$(git -C "$tmp/repo.git" show HEAD:go.mod 2>/dev/null | awk '/^module / {print $2; exit}')
  echo "devpod-backend: HEAD $head module ${mod:-unknown}"
  git fetch --quiet "$tmp/repo.git" HEAD
  unique=$(git rev-list --count HEAD..FETCH_HEAD)
  echo "devpod-backend: $unique commit(s) are not in this branch."
  if [[ "$unique" != 0 ]]; then
    git rev-list --oneline HEAD..FETCH_HEAD | head -50
    echo "Apply those commits onto the repository root. Do not subtree-add a second CLI."
    note_failure "devpod-backend: unique commits need review"
  else
    echo "devpod-backend: history is already contained in HEAD."
  fi
  rm -rf "$tmp"
}

import_one() {
  local name=$1 url=$2 prefix=$3 branch=$4
  if [[ -e "$prefix" ]]; then
    echo "$name: $prefix already exists; leaving it in place"
    return 0
  fi
  local resolved
  if ! resolved=$(resolve_branch "$url" "$branch"); then
    note_failure "$name: could not resolve a branch at $url"
    return 0
  fi
  echo "$name: default-or-requested branch is $resolved"
  if [[ "$mode" == dry-run ]]; then
    return 0
  fi
  echo "$name: git subtree add --prefix=$prefix $url $resolved"
  if ! git subtree add --prefix="$prefix" --message "chore: import ${url} into ${prefix}" "$url" "$resolved"; then
    note_failure "$name: subtree add failed"
    return 0
  fi
}

mapfile -t rows < <(python3 - <<'PY'
import json
m=json.load(open("monorepo.manifest.json"))
for s in m["satellites"]:
    policy=s.get("import")
    if policy == "skip-duplicate-cli":
        print("\t".join(["backend", s["url"], "", ""]))
        continue
    if policy != "subtree":
        continue
    print("\t".join([
        s["name"],
        s["url"],
        s["prefix"],
        s.get("branch") or "",
    ]))
PY
)

for row in "${rows[@]}"; do
  IFS=$'\t' read -r name url prefix branch <<<"$row"
  if [[ "$name" == backend ]]; then
    inspect_backend "$url"
    continue
  fi
  import_one "$name" "$url" "$prefix" "$branch"
done

if [[ "$mode" == import ]]; then
  python3 hack/sync-workspaces.py --write
  if [[ -n "$(git status --porcelain)" ]]; then
    git add Cargo.toml package.json
    git commit -m "chore: sync workspace members after satellite import"
  fi
fi

if [[ "$failures" -ne 0 ]]; then
  echo "$failures satellite import(s) still open. Satellite remotes were not modified." >&2
  exit 1
fi
echo "satellite import finished. Satellite remotes were not modified."
