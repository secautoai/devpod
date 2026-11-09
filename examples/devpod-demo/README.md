# DevPod Demo

A minimal repository for testing the [DevPod](https://devpod.sh) CLI binary.

## Purpose

Use this repo to verify DevPod works end-to-end:

- `devpod up` (create/start workspace)
- `devpod ssh` (shell into workspace)
- `devpod list` / `devpod status` / `devpod stop` / `devpod delete`

## Prerequisites

- DevPod CLI installed and in `PATH`
- A provider configured (e.g. Docker):  
  `devpod provider add docker`  
  `devpod provider use docker`

## Quick test (local path)

```bash
# From this repo directory
devpod up . --ide none
devpod ssh .
# ... use the workspace, then:
devpod stop .
```

## Test from Git URL (after pushing to remote)

```bash
devpod up https://github.com/YOUR_ORG/devpod-demo --ide none
devpod ssh devpod-demo
devpod list
devpod status devpod-demo
devpod stop devpod-demo
devpod delete devpod-demo
```

## Structure

- `.devcontainer/devcontainer.json` — devcontainer config (Ubuntu base image)
- No app code required; the container is enough to confirm DevPod works.

## Push to remote

After cloning or creating this repo locally:

```bash
git remote add origin https://github.com/YOUR_ORG/devpod-demo.git
git push -u origin main
```

Then use the Git URL with `devpod up` as above.
