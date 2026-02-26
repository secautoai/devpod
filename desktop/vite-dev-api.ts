/**
 * Vite plugin that provides the DevPod web API endpoints during development.
 *
 * When `yarn dev` is run without the Go backend (`devpod web`) running, this
 * plugin handles `/api/*` and `/releases` routes directly inside the Vite dev
 * server.  It spawns `devpod` commands via child_process, mirroring what the
 * Go server in `cmd/web/web.go` does.
 *
 * If the Go backend IS running, the Vite proxy (configured in vite.config.ts)
 * takes precedence because proxy rules are evaluated before server middleware.
 */

import { type Plugin } from "vite"
import { type IncomingMessage, type ServerResponse } from "node:http"
import { spawn, execFile, execSync } from "node:child_process"
import { platform, arch } from "node:os"
import { resolve } from "node:path"
import { existsSync } from "node:fs"
import { WebSocketServer, type WebSocket } from "ws"

const DEVPOD_NOT_FOUND =
  "DevPod CLI not found. Install it, add it to PATH, or set DEVPOD_BIN to the full path."

// Try to find the devpod binary – env, then repo-relative paths, then PATH
function findDevpod(): string {
  if (process.env.DEVPOD_BIN) return process.env.DEVPOD_BIN

  // Prefer repo-relative paths (e.g. go build -o devpod . in repo root)
  const candidates = [
    resolve(__dirname, "..", "devpod"),       // desktop/../devpod
    resolve(process.cwd(), "..", "devpod"),   // cwd/../devpod (e.g. when cwd is desktop)
    resolve(process.cwd(), "devpod"),        // cwd/devpod (e.g. when cwd is repo root)
  ]
  for (const p of candidates) {
    if (existsSync(p)) return p
  }

  try {
    const cmd = platform() === "win32" ? "where devpod" : "command -v devpod"
    const out = execSync(cmd, { encoding: "utf8", stdio: ["pipe", "pipe", "ignore"] })
    const first = out.trim().split(/\r?\n/)[0]?.trim()
    if (first) return first
  } catch {
    // which/where failed
  }

  return "devpod"
}

function jsonResponse(res: ServerResponse, data: unknown, status = 200) {
  res.writeHead(status, { "Content-Type": "application/json" })
  res.end(JSON.stringify(data))
}

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    let body = ""
    req.on("data", (chunk: Buffer) => (body += chunk.toString()))
    req.on("end", () => resolve(body))
    req.on("error", reject)
  })
}

export default function devApiPlugin(): Plugin {
  const devpod = findDevpod()

  return {
    name: "devpod-dev-api",
    apply: "serve", // only active during `vite dev`, not during build

    configureServer(server) {
      // ---- REST endpoints (evaluated before Vite's own middleware) ----
      server.middlewares.use(async (req, res, next) => {
        const url = req.url ?? ""

        // /api/health
        if (url === "/api/health") {
          res.writeHead(200)
          res.end("OK")
          return
        }

        // /api/version
        if (url === "/api/version") {
          jsonResponse(res, { version: process.env.DEVPOD_VERSION ?? "dev" })
          return
        }

        // /api/platform
        if (url === "/api/platform") {
          jsonResponse(res, { platform: platform(), arch: arch() })
          return
        }

        // /releases
        if (url === "/releases") {
          jsonResponse(res, [])
          return
        }

        // /api/providers  (convenience endpoint)
        if (url === "/api/providers" && req.method === "GET") {
          runJSONSubcommand(res, ["provider", "list", "--output=json", "--log-output=json"])
          return
        }

        // /api/workspaces (convenience endpoint)
        if (url === "/api/workspaces" && req.method === "GET") {
          runJSONSubcommand(res, ["list", "--output=json", "--log-output=json"])
          return
        }

        // /api/command (synchronous command execution)
        if (url === "/api/command" && req.method === "POST") {
          try {
            const body = JSON.parse(await readBody(req)) as {
              args: string[]
              env?: Record<string, string>
            }
            runCommand(res, body.args, body.env ?? {})
          } catch {
            res.writeHead(400, { "Content-Type": "text/plain" })
            res.end("invalid request body")
          }
          return
        }

        // /api/signal
        if (url === "/api/signal" && req.method === "POST") {
          // Signal handling is best-effort in dev mode
          res.writeHead(200)
          res.end()
          return
        }

        // Not an API route – pass through to Vite
        next()
      })

      // ---- WebSocket endpoint for streaming ----
      const wss = new WebSocketServer({ noServer: true })

      server.httpServer?.on("upgrade", (req, socket, head) => {
        if (req.url === "/api/command/stream") {
          wss.handleUpgrade(req, socket, head, (ws) => {
            handleStreamConnection(ws)
          })
        }
        // Other upgrade requests (like Vite HMR) are left alone
      })
    },
  }

  // ---- Helper: run a devpod sub-command and return JSON stdout ----
  function runJSONSubcommand(res: ServerResponse, args: string[]) {
    execFile(devpod, args, { timeout: 30_000 }, (err, stdout, stderr) => {
      if (err) {
        const msg = err.code === "ENOENT" ? DEVPOD_NOT_FOUND : stderr || err.message
        res.writeHead(err.code === "ENOENT" ? 503 : 502, { "Content-Type": "text/plain" })
        res.end(msg)
        return
      }
      res.writeHead(200, { "Content-Type": "application/json" })
      res.end(stdout)
    })
  }

  // ---- Helper: run a command and return { stdout, stderr, code } ----
  function runCommand(
    res: ServerResponse,
    args: string[],
    env: Record<string, string>
  ) {
    const childEnv = { ...process.env, ...env }
    execFile(devpod, args, { env: childEnv, timeout: 60_000 }, (err, stdout, stderr) => {
      if (err) {
        const exitCode = typeof err.code === "number" ? err.code : 1
        const stderrMsg = err.code === "ENOENT" ? DEVPOD_NOT_FOUND : (stderr || err.message)
        jsonResponse(res, {
          stdout: stdout ?? "",
          stderr: stderrMsg,
          code: exitCode,
        })
        return
      }
      jsonResponse(res, { stdout, stderr, code: 0 })
    })
  }

  // ---- Helper: handle streaming WebSocket connection ----
  function handleStreamConnection(ws: WebSocket) {
    const processes = new Map<string, ReturnType<typeof spawn>>()

    ws.on("message", (raw: Buffer) => {
      let msg: { type: string; id: string; args?: string[]; env?: Record<string, string> }
      try {
        msg = JSON.parse(raw.toString())
      } catch {
        ws.send(JSON.stringify({ type: "error", data: "invalid message" }))
        return
      }

      if (msg.type === "start" && msg.id && msg.args) {
        const childEnv = { ...process.env, ...(msg.env ?? {}) }
        let child: ReturnType<typeof spawn>
        try {
          child = spawn(devpod, msg.args, { env: childEnv })
        } catch (err) {
          const message = err && typeof err === "object" && "code" in err && (err as NodeJS.ErrnoException).code === "ENOENT"
            ? DEVPOD_NOT_FOUND
            : (err instanceof Error ? err.message : String(err))
          ws.send(JSON.stringify({ type: "error", id: msg.id, data: message }))
          return
        }
        processes.set(msg.id, child)

        child.stdout?.on("data", (data: Buffer) => {
          ws.send(JSON.stringify({ type: "stdout", id: msg.id, data: data.toString() }))
        })

        child.stderr?.on("data", (data: Buffer) => {
          ws.send(JSON.stringify({ type: "stderr", id: msg.id, data: data.toString() }))
        })

        child.on("close", (code) => {
          processes.delete(msg.id)
          ws.send(JSON.stringify({ type: "exit", id: msg.id, exitCode: code ?? 0 }))
        })

        child.on("error", (err) => {
          processes.delete(msg.id)
          const message = (err as NodeJS.ErrnoException).code === "ENOENT" ? DEVPOD_NOT_FOUND : err.message
          ws.send(JSON.stringify({ type: "error", id: msg.id, data: message }))
        })
      } else if (msg.type === "cancel" && msg.id) {
        const child = processes.get(msg.id)
        if (child) {
          child.kill("SIGINT")
        }
      }
    })

    ws.on("close", () => {
      // Clean up any running processes
      processes.forEach((child) => child.kill("SIGINT"))
      processes.clear()
    })
  }
}
