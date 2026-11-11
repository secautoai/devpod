#!/usr/bin/env python3
"""Align Cargo members and npm workspaces with imported satellite directories.

The allow-list is monorepo.manifest.json. desktop/ is a Yarn project and is
never an npm workspace. desktop/src-tauri is never a Cargo workspace member.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "monorepo.manifest.json"
CARGO = ROOT / "Cargo.toml"
PACKAGE = ROOT / "package.json"


def load_manifest() -> dict:
    return json.loads(MANIFEST.read_text())


def present(satellites: list[dict], flag: str, marker: str) -> list[str]:
    found: list[str] = []
    for sat in satellites:
        prefix = sat.get("prefix")
        if not prefix or not sat.get(flag):
            continue
        if (ROOT / prefix / marker).is_file():
            found.append(prefix)
    return found


def render_members(members: list[str]) -> str:
    body = "\n".join(f'  "{member}",' for member in members)
    if body:
        body += "\n"
    return f"members = [\n{body}]"


def cargo_text(members: list[str]) -> str:
    text = CARGO.read_text()
    start = text.find("members = [")
    if start < 0:
        raise SystemExit("Cargo.toml is missing a members array")
    end = text.find("]", start)
    if end < 0:
        raise SystemExit("Cargo.toml members array is not closed")
    return text[:start] + render_members(members) + text[end + 1 :]


def package_text(workspaces: list[str]) -> str:
    data = json.loads(PACKAGE.read_text())
    data["workspaces"] = workspaces
    return json.dumps(data, indent=2) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="exit 1 if files differ")
    parser.add_argument("--write", action="store_true", help="rewrite workspace files")
    parser.add_argument("--list", choices=("rust", "js"), help="print present prefixes")
    args = parser.parse_args()

    satellites = load_manifest()["satellites"]
    rust = present(satellites, "cargo", "Cargo.toml")
    js = present(satellites, "npm", "package.json")

    if args.list == "rust":
        print("\n".join(rust))
        return 0
    if args.list == "js":
        print("\n".join(js))
        return 0

    cargo_next = cargo_text(rust)
    package_next = package_text(js)
    cargo_changed = cargo_next != CARGO.read_text()
    package_changed = package_next != PACKAGE.read_text()

    if args.write:
        if cargo_changed:
            CARGO.write_text(cargo_next)
        if package_changed:
            PACKAGE.write_text(package_next)
        print(f"rust members: {rust or '(none)'}")
        print(f"npm workspaces: {js or '(none)'}")
        return 0

    if args.check or not (args.write or args.list):
        if cargo_changed or package_changed:
            print("workspace files differ from imported satellites", file=sys.stderr)
            print(f"expected rust: {rust or '(none)'}", file=sys.stderr)
            print(f"expected npm: {js or '(none)'}", file=sys.stderr)
            return 1
        print(f"rust members: {rust or '(none)'}")
        print(f"npm workspaces: {js or '(none)'}")
        return 0

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
