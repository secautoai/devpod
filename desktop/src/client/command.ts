import {
  Child,
  ChildProcess,
  EventEmitter,
  Command as ShellCommand,
} from "@tauri-apps/plugin-shell"
import { debug, ErrorTypeCancelled, isError, Result, ResultError, Return, sleep } from "@/lib"
import { IS_TAURI, WEB_SERVER_URL } from "@/lib/platform"
import {
  DEVPOD_BINARY,
  DEVPOD_FLAG_OPTION,
  DEVPOD_UI_ENV_VAR,
  DEVPOD_ADDITIONAL_ENV_VARS,
} from "./constants"
import { TStreamEvent } from "./types"
import { TAURI_SERVER_URL } from "./tauriClient"
import { invoke } from "@tauri-apps/api/core"
import { WebCommand } from "./webClient/command"

export type TStreamEventListenerFn = (event: TStreamEvent) => void
export type TEventListener<TEventName extends string> = Parameters<
  EventEmitter<Record<TEventName, string>>["addListener"]
>[1]
type TStreamOptions = Readonly<{
  ignoreStdoutError?: boolean
  ignoreStderrError?: boolean
}>
const defaultStreamOptions: TStreamOptions = {
  ignoreStdoutError: false,
  ignoreStderrError: false,
}

export type TCommand<T> = {
  run(): Promise<Result<T>>
  stream(listener: TStreamEventListenerFn): Promise<ResultError>
  cancel(): Promise<ResultError>
}

// Shared type for command results - ChildProcess in Tauri mode, compatible struct in web mode
export type TCommandResult = ChildProcess<string> | { stdout: string; stderr: string; code: number }

export class Command implements TCommand<TCommandResult> {
  private sidecarCommand!: ShellCommand<string>
  private childProcess?: Child
  private args: string[]
  private cancelled = false
  private isFlatpak: boolean | undefined
  private extraEnvVars: Record<string, string>

  // Web-mode delegate
  private webCommand: WebCommand | null = null

  public static ADDITIONAL_ENV_VARS: string = ""
  public static HTTP_PROXY: string = ""
  public static HTTPS_PROXY: string = ""
  public static NO_PROXY: string = ""

  constructor(args: string[]) {
    debug("commands", "Creating Devpod command with args: ", args)
    this.args = args
    this.extraEnvVars = Command.ADDITIONAL_ENV_VARS.split(",")
      .map((envVarStr) => envVarStr.split("="))
      .reduce(
        (acc, pair) => {
          const [key, value] = pair
          if (key === undefined || value === undefined) {
            return acc
          }

          return { ...acc, [key]: value }
        },
        {} as Record<string, string>
      )

    // set proxy related environment variables
    if (Command.HTTP_PROXY) {
      this.extraEnvVars["HTTP_PROXY"] = Command.HTTP_PROXY
    }
    if (Command.HTTPS_PROXY) {
      this.extraEnvVars["HTTPS_PROXY"] = Command.HTTPS_PROXY
    }
    if (Command.NO_PROXY) {
      this.extraEnvVars["NO_PROXY"] = Command.NO_PROXY
    }

    // allows the CLI to detect if commands have been invoked from the UI
    this.extraEnvVars[DEVPOD_UI_ENV_VAR] = "true"

    if (IS_TAURI) {
      this.sidecarCommand = ShellCommand.sidecar(DEVPOD_BINARY, args, { env: this.extraEnvVars })
    } else {
      // Sync static flags to WebCommand
      WebCommand.ADDITIONAL_ENV_VARS = Command.ADDITIONAL_ENV_VARS
      WebCommand.HTTP_PROXY = Command.HTTP_PROXY
      WebCommand.HTTPS_PROXY = Command.HTTPS_PROXY
      WebCommand.NO_PROXY = Command.NO_PROXY
      this.webCommand = new WebCommand(args)
    }
  }

  public async getEnv(name: string): Promise<boolean> {
    if (!IS_TAURI) return false
    return invoke<boolean>("get_env", { name })
  }

  public async run(): Promise<Result<TCommandResult>> {
    if (!IS_TAURI && this.webCommand) {
      return this.webCommand.run()
    }

    try {
      // Run once to check with the rust backend if we are running inside the flatpak sandbox
      // This informs the CLI wrapper to use flatpak-spawn to escape the sandbox and export this.extraEnvVars
      if (this.isFlatpak === undefined) {
        this.isFlatpak = await this.getEnv("FLATPAK_ID")
        if (this.isFlatpak) {
          this.extraEnvVars["FLATPAK_ID"] = "sh.loft.devpod"
          this.extraEnvVars[DEVPOD_ADDITIONAL_ENV_VARS] = recordToCSV(this.extraEnvVars)
          this.sidecarCommand = ShellCommand.sidecar(DEVPOD_BINARY, this.args, {
            env: this.extraEnvVars,
          })
        }
      }
      const rawResult = await this.sidecarCommand.execute()
      debug("commands", `Result for command with args ${this.args}:`, rawResult)

      return Return.Value(rawResult)
    } catch (e) {
      return Return.Failed(e + "")
    }
  }

  public async stream(
    listener: TStreamEventListenerFn,
    streamOptions?: TStreamOptions
  ): Promise<ResultError> {
    if (!IS_TAURI && this.webCommand) {
      return this.webCommand.stream(listener)
    }

    let opts = defaultStreamOptions
    if (streamOptions) {
      opts = { ...defaultStreamOptions, ...streamOptions }
    }

    try {
      this.childProcess = await this.sidecarCommand.spawn()
      if (this.cancelled) {
        await this.childProcess.kill()

        return Return.Failed("Command already cancelled", "", ErrorTypeCancelled)
      }

      await new Promise((res, rej) => {
        const stdoutListener: TEventListener<"data"> = (message) => {
          try {
            const data = JSON.parse(message)

            // special case: the cli sends us a message where "done" is "true"
            // to signal the command is terminated and we should stop listen to it
            // This happens for the vscode browser command as it needs to stay open
            // for port-forwarding, but we don't care anymore about its output.
            if (data?.done === "true") {
              res(Return.Ok())
            } else {
              listener({ type: "data", data })
            }
          } catch (error) {
            if (!opts.ignoreStdoutError) {
              console.error("Failed to parse stdout message ", message, error)
            }
          }
        }
        const stderrListener: TEventListener<"data"> = (message) => {
          try {
            const error = JSON.parse(message)
            listener({ type: "error", error })
          } catch (error) {
            if (!opts.ignoreStderrError) {
              console.error("Failed to parse stderr message ", message, error)
            }
          }
        }

        this.sidecarCommand.stderr.addListener("data", stderrListener)
        this.sidecarCommand.stdout.addListener("data", stdoutListener)

        const cleanup = () => {
          this.sidecarCommand.stderr.removeListener("data", stderrListener)
          this.sidecarCommand.stdout.removeListener("data", stdoutListener)
          this.childProcess = undefined
        }

        this.sidecarCommand.on("close", ({ code }) => {
          cleanup()
          if (code !== 0) {
            rej(new Error("exit code: " + code))
          } else {
            res(Return.Ok())
          }
        })

        this.sidecarCommand.on("error", (arg) => {
          cleanup()
          rej(arg)
        })
      })

      return Return.Ok()
    } catch (e) {
      if (isError(e)) {
        if (this.cancelled) {
          return Return.Failed(e.message, "", ErrorTypeCancelled)
        }

        return Return.Failed(e.message)
      }
      console.error(e)

      return Return.Failed("streaming failed")
    }
  }

  /**
   * Cancel the command.
   * Only works if it has been created with the `stream` method.
   */
  public async cancel(): Promise<Result<undefined>> {
    if (!IS_TAURI && this.webCommand) {
      return this.webCommand.cancel()
    }

    try {
      this.cancelled = true
      if (!this.childProcess) {
        // nothing to clean up
        return Return.Ok()
      }
      // Try to send signal first before force killing process
      await fetch(TAURI_SERVER_URL + "/child-process/signal", {
        method: "POST",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({
          processId: this.childProcess.pid,
          signal: 2, // SIGINT
        }),
      })

      await sleep(3_000)
      // the actual child process could be gone after sending a SIGINT
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
      if (this.childProcess) {
        await this.childProcess.kill()
      }

      return Return.Ok()
    } catch (e) {
      if (isError(e)) {
        return Return.Failed(e.message)
      }

      return Return.Failed("failed to cancel command")
    }
  }
}

export function isOk(result: TCommandResult): boolean {
  return result.code === 0
}

export function toFlagArg(flag: string, arg: string) {
  return [flag, arg].join("=")
}

export function serializeRawOptions(
  rawOptions: Record<string, unknown>,
  flag: string = DEVPOD_FLAG_OPTION
): string[] {
  return Object.entries(rawOptions).map(([key, value]) => flag + `=${key}=${value}`)
}

function recordToCSV(record: Record<string, string>): string {
  return Object.entries(record)
    .map(([key, value]) => `${key}=${value}`)
    .join(",")
}
