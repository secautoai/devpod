/**
 * WebCommand – a drop-in replacement for the Tauri-based Command class.
 *
 * Instead of spawning the devpod sidecar via Tauri's shell plugin, it opens
 * a WebSocket connection to the `devpod web` HTTP server and asks it to run
 * the command there.  stdout/stderr are streamed back line-by-line.
 *
 * The public API matches the TCommand<ChildProcess> interface used by the
 * existing WorkspaceCommands and other callers so no changes are required
 * in the business logic layer.
 */

import { WEB_SERVER_URL, WEB_SERVER_WS_URL } from "@/lib/platform"
import { ErrorTypeCancelled, isError, Result, ResultError, Return, sleep } from "@/lib"
import { DEVPOD_UI_ENV_VAR } from "../constants"
import { TStreamEventListenerFn } from "../types"

export type ChildProcessResult = {
  stdout: string
  stderr: string
  code: number
}

type WsOutgoing =
  | { type: "start"; id: string; args: string[]; env: Record<string, string> }
  | { type: "cancel"; id: string }

type WsIncoming =
  | { type: "stdout"; id: string; data: string }
  | { type: "stderr"; id: string; data: string }
  | { type: "exit"; id: string; exitCode: number }
  | { type: "error"; id: string; data: string }

// Shared WebSocket connection (lazily created, auto-reconnected)
let sharedWs: WebSocket | null = null
const pendingCallbacks = new Map<
  string,
  {
    onStdout: (data: string) => void
    onStderr: (data: string) => void
    onExit: (code: number) => void
    onError: (msg: string) => void
  }
>()

function getSharedWs(): Promise<WebSocket> {
  return new Promise((resolve, reject) => {
    if (sharedWs && sharedWs.readyState === WebSocket.OPEN) {
      resolve(sharedWs)
      return
    }

    const ws = new WebSocket(WEB_SERVER_WS_URL)
    sharedWs = ws

    ws.onopen = () => resolve(ws)
    ws.onerror = (e) => reject(new Error("WebSocket connection failed"))
    ws.onclose = () => {
      sharedWs = null
      // Notify any remaining commands that the connection closed
      for (const [id, cbs] of pendingCallbacks) {
        cbs.onError("WebSocket connection closed unexpectedly")
        pendingCallbacks.delete(id)
      }
    }
    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data) as WsIncoming
        const cbs = pendingCallbacks.get(msg.id)
        if (!cbs) return

        switch (msg.type) {
          case "stdout":
            cbs.onStdout(msg.data)
            break
          case "stderr":
            cbs.onStderr(msg.data)
            break
          case "exit":
            cbs.onExit(msg.exitCode)
            pendingCallbacks.delete(msg.id)
            break
          case "error":
            cbs.onError(msg.data)
            pendingCallbacks.delete(msg.id)
            break
        }
      } catch {
        // ignore parse errors
      }
    }
  })
}

let _idCounter = 0
function nextId(): string {
  return `cmd-${Date.now()}-${++_idCounter}`
}

export class WebCommand {
  private args: string[]
  private id: string = nextId()
  private cancelled = false
  private env: Record<string, string> = {}

  public static ADDITIONAL_ENV_VARS: string = ""
  public static HTTP_PROXY: string = ""
  public static HTTPS_PROXY: string = ""
  public static NO_PROXY: string = ""

  constructor(args: string[]) {
    this.args = args
    this.env = {
      [DEVPOD_UI_ENV_VAR]: "true",
    }
    if (WebCommand.HTTP_PROXY) this.env["HTTP_PROXY"] = WebCommand.HTTP_PROXY
    if (WebCommand.HTTPS_PROXY) this.env["HTTPS_PROXY"] = WebCommand.HTTPS_PROXY
    if (WebCommand.NO_PROXY) this.env["NO_PROXY"] = WebCommand.NO_PROXY

    // Parse ADDITIONAL_ENV_VARS = "KEY=VAL,KEY2=VAL2"
    if (WebCommand.ADDITIONAL_ENV_VARS) {
      for (const pair of WebCommand.ADDITIONAL_ENV_VARS.split(",")) {
        const idx = pair.indexOf("=")
        if (idx > 0) {
          this.env[pair.slice(0, idx)] = pair.slice(idx + 1)
        }
      }
    }
  }

  /** Run the command synchronously (collects all stdout/stderr). */
  public async run(): Promise<Result<ChildProcessResult>> {
    try {
      const res = await fetch(WEB_SERVER_URL + "/api/command", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ args: this.args, env: this.env }),
      })
      if (!res.ok) {
        return Return.Failed(`Server error: ${res.statusText}`)
      }
      const data = (await res.json()) as ChildProcessResult
      return Return.Value(data)
    } catch (e) {
      return Return.Failed(isError(e) ? e.message : String(e))
    }
  }

  /** Stream command output to a listener function. */
  public async stream(listener: TStreamEventListenerFn): Promise<ResultError> {
    try {
      const ws = await getSharedWs()
      this.id = nextId()

      return await new Promise<ResultError>((resolve) => {
        let stdoutBuf = ""
        let stderrBuf = ""

        const tryEmitStdout = (chunk: string) => {
          stdoutBuf += chunk
          // Emit complete JSON lines
          const lines = stdoutBuf.split("\n")
          stdoutBuf = lines.pop() ?? ""
          for (const line of lines) {
            const trimmed = line.trim()
            if (!trimmed) continue
            try {
              const data = JSON.parse(trimmed)
              if (data?.done === "true") {
                resolve(Return.Ok())
                return
              }
              listener({ type: "data", data })
            } catch {
              // non-JSON output – pass as raw message
              listener({ type: "data", data: { message: trimmed, level: "info" } as any })
            }
          }
        }

        const tryEmitStderr = (chunk: string) => {
          stderrBuf += chunk
          const lines = stderrBuf.split("\n")
          stderrBuf = lines.pop() ?? ""
          for (const line of lines) {
            const trimmed = line.trim()
            if (!trimmed) continue
            try {
              const error = JSON.parse(trimmed)
              listener({ type: "error", error })
            } catch {
              listener({ type: "error", error: { message: trimmed, level: "error" } as any })
            }
          }
        }

        pendingCallbacks.set(this.id, {
          onStdout: tryEmitStdout,
          onStderr: tryEmitStderr,
          onExit: (code) => {
            if (code !== 0) {
              resolve(Return.Failed(`exit code: ${code}`))
            } else {
              resolve(Return.Ok())
            }
          },
          onError: (msg) => {
            if (this.cancelled) {
              resolve(Return.Failed(msg, "", ErrorTypeCancelled))
            } else {
              resolve(Return.Failed(msg))
            }
          },
        })

        const msg: WsOutgoing = {
          type: "start",
          id: this.id,
          args: this.args,
          env: this.env,
        }
        ws.send(JSON.stringify(msg))
      })
    } catch (e) {
      return Return.Failed(isError(e) ? e.message : "streaming failed")
    }
  }

  /** Cancel a running command. */
  public async cancel(): Promise<ResultError> {
    this.cancelled = true
    try {
      if (sharedWs && sharedWs.readyState === WebSocket.OPEN) {
        const msg: WsOutgoing = { type: "cancel", id: this.id }
        sharedWs.send(JSON.stringify(msg))
      }
      // Also try via signal endpoint using process signal
      await sleep(3_000)
      return Return.Ok()
    } catch (e) {
      return Return.Failed(isError(e) ? e.message : "cancel failed")
    }
  }
}
