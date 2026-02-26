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

// ─── Dev-mode in-memory store ─────────────────────────────────────────────────

const DEV_TOKEN = "dev-mock-token"

type MockUser = { id: string; username: string; role: "admin" | "user"; dockerHost: string; createdAt: string; password: string }

const mockUsers: Map<string, MockUser> = new Map([
  ["admin", { id: "1", username: "admin", role: "admin", dockerHost: "", createdAt: new Date().toISOString(), password: "admin" }],
])

let mockBranding = {
  appName: "DevPod",
  logoUrl: "",
  faviconUrl: "",
  providerDownloadUrl: "https://github.com/loft-sh/devpod/releases",
  supportUrl: "",
  docsUrl: "https://devpod.sh/docs",
  primaryColor: "",
}

const mockProviderPerms: Map<string, string[] | null> = new Map()
const mockOpLogs: Array<{ id: string; username: string; action: string; target: string; args: string[]; startedAt: string; exitCode: number; error: string }> = []

// ─── Helpers ──────────────────────────────────────────────────────────────────

function userPublic(u: MockUser) {
  return { id: u.id, username: u.username, role: u.role, dockerHost: u.dockerHost, createdAt: u.createdAt }
}

const DEVPOD_NOT_FOUND =
  "DevPod CLI not found. Install it, add it to PATH, or set DEVPOD_BIN to the full path."

// Try to find the devpod binary – check env, PATH, then repo-relative path
function findDevpod(): string {
  if (process.env.DEVPOD_BIN) return process.env.DEVPOD_BIN

  try {
    const cmd = platform() === "win32" ? "where devpod" : "command -v devpod"
    const out = execSync(cmd, { encoding: "utf8", stdio: ["pipe", "pipe", "ignore"] })
    const first = out.trim().split(/\r?\n/)[0]?.trim()
    if (first) return first
  } catch {
    // which/where failed – try repo-relative path (e.g. go build in repo root)
  }

  const repoBinary = resolve(__dirname, "..", "devpod")
  if (existsSync(repoBinary)) return repoBinary

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

        // ── Auth endpoints (dev mocks) ──────────────────────────────────────

        // POST /api/login
        if (url === "/api/login" && req.method === "POST") {
          try {
            const body = JSON.parse(await readBody(req)) as { username?: string; password?: string }
            const user = mockUsers.get(body.username ?? "")
            if (!user || user.password !== body.password) {
              res.writeHead(401, { "Content-Type": "text/plain" })
              res.end("invalid credentials")
              return
            }
            jsonResponse(res, { token: DEV_TOKEN, user: userPublic(user) })
          } catch {
            res.writeHead(400, { "Content-Type": "text/plain" })
            res.end("invalid request body")
          }
          return
        }

        // GET /api/me
        if (url === "/api/me" && req.method === "GET") {
          const auth = req.headers["authorization"] ?? ""
          if (!auth.startsWith("Bearer ")) { res.writeHead(401, { "Content-Type": "text/plain" }); res.end("unauthorized"); return }
          const user = mockUsers.get("admin")!
          jsonResponse(res, userPublic(user))
          return
        }

        // GET /api/branding  (public)
        if (url === "/api/branding" && req.method === "GET") {
          jsonResponse(res, mockBranding)
          return
        }

        // PUT /api/admin/branding
        if (url === "/api/admin/branding" && req.method === "PUT") {
          try {
            const partial = JSON.parse(await readBody(req))
            mockBranding = { ...mockBranding, ...partial }
            jsonResponse(res, mockBranding)
          } catch { res.writeHead(400).end("bad request") }
          return
        }

        // DELETE /api/admin/branding
        if (url === "/api/admin/branding" && req.method === "DELETE") {
          mockBranding = { appName: "DevPod", logoUrl: "", faviconUrl: "", providerDownloadUrl: "https://github.com/loft-sh/devpod/releases", supportUrl: "", docsUrl: "https://devpod.sh/docs", primaryColor: "" }
          jsonResponse(res, mockBranding)
          return
        }

        // GET /api/admin/users  /  POST /api/admin/users
        if (url === "/api/admin/users" && (req.method === "GET" || req.method === "POST")) {
          if (req.method === "GET") {
            jsonResponse(res, [...mockUsers.values()].map(userPublic))
          } else {
            try {
              const body = JSON.parse(await readBody(req)) as { username: string; password: string; role: "admin" | "user" }
              if (mockUsers.has(body.username)) { res.writeHead(409, { "Content-Type": "text/plain" }); res.end("username already exists"); return }
              const newUser: MockUser = { id: String(Date.now()), username: body.username, role: body.role ?? "user", dockerHost: "", createdAt: new Date().toISOString(), password: body.password }
              mockUsers.set(body.username, newUser)
              res.writeHead(201, { "Content-Type": "application/json" }); res.end(JSON.stringify(userPublic(newUser)))
            } catch { res.writeHead(400).end("bad request") }
          }
          return
        }

        // PUT|DELETE /api/admin/users/:username
        const adminUserMatch = url.match(/^\/api\/admin\/users\/([^/]+)$/)
        if (adminUserMatch) {
          const uname = decodeURIComponent(adminUserMatch[1])
          if (req.method === "PUT") {
            try {
              const body = JSON.parse(await readBody(req)) as { role?: "admin" | "user"; dockerHost?: string }
              const u = mockUsers.get(uname)
              if (!u) { res.writeHead(404).end("not found"); return }
              if (body.role) u.role = body.role
              if (body.dockerHost !== undefined) u.dockerHost = body.dockerHost
              jsonResponse(res, userPublic(u))
            } catch { res.writeHead(400).end("bad request") }
          } else if (req.method === "DELETE") {
            mockUsers.delete(uname)
            res.writeHead(204).end()
          } else { res.writeHead(405).end() }
          return
        }

        // GET|PUT /api/admin/users/:username/providers
        const adminProviderMatch = url.match(/^\/api\/admin\/users\/([^/]+)\/providers$/)
        if (adminProviderMatch) {
          const uname = decodeURIComponent(adminProviderMatch[1])
          if (req.method === "GET") {
            const perms = mockProviderPerms.get(uname)
            jsonResponse(res, perms === undefined ? { unrestricted: true, providers: null } : { unrestricted: false, providers: perms })
          } else if (req.method === "PUT") {
            try {
              const body = JSON.parse(await readBody(req)) as { providers: string[] | null }
              mockProviderPerms.set(uname, body.providers)
              res.writeHead(204).end()
            } catch { res.writeHead(400).end("bad request") }
          } else { res.writeHead(405).end() }
          return
        }

        // GET /api/admin/logs
        if (url.startsWith("/api/admin/logs") && req.method === "GET") {
          jsonResponse(res, mockOpLogs)
          return
        }

        // GET /api/logs
        if (url.startsWith("/api/logs") && req.method === "GET") {
          jsonResponse(res, mockOpLogs)
          return
        }

        // POST /api/user/password
        if (url === "/api/user/password" && req.method === "POST") {
          try {
            const body = JSON.parse(await readBody(req)) as { currentPassword: string; newPassword: string }
            const u = mockUsers.get("admin")!
            if (u.password !== body.currentPassword) { res.writeHead(401, { "Content-Type": "text/plain" }); res.end("current password incorrect"); return }
            u.password = body.newPassword
            res.writeHead(204).end()
          } catch { res.writeHead(400).end("bad request") }
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
