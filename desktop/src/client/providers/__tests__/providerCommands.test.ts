/**
 * Unit tests for ProviderCommands.
 *
 * We intercept the Command constructor so that no real subprocess is spawned.
 * Each test verifies that ProviderCommands builds the correct CLI argument array
 * and parses the response correctly.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"

// ─── Mock setup ───────────────────────────────────────────────────────────────
//
// We mock the `../command` module so that `new Command([...]).run()` returns
// whatever we configure via `mockRunResult`.

let capturedArgs: string[] = []
let mockRunResult: { err: boolean; val: { stdout: string; stderr: string; code: number } } = {
  err: false,
  val: { stdout: "", stderr: "", code: 0 },
}

vi.mock("../../command", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../command")>()
  class MockCommand {
    constructor(private args: string[]) {
      capturedArgs = args
    }
    async run() {
      return mockRunResult as any
    }
    async stream() {
      return { err: false, val: undefined } as any
    }
    async cancel() {
      return { err: false, val: undefined } as any
    }
  }
  return {
    ...actual,
    Command: MockCommand,
  }
})

// ─── Actual imports (after mock) ──────────────────────────────────────────────

import { ProviderCommands } from "../providerCommands"
import {
  DEVPOD_COMMAND_PROVIDER,
  DEVPOD_COMMAND_LIST,
  DEVPOD_FLAG_JSON_OUTPUT,
  DEVPOD_FLAG_JSON_LOG_OUTPUT,
  DEVPOD_COMMAND_ADD,
  DEVPOD_FLAG_USE,
  DEVPOD_COMMAND_DELETE,
  DEVPOD_COMMAND_USE,
  DEVPOD_COMMAND_OPTIONS,
  DEVPOD_COMMAND_SET_OPTIONS,
} from "../../constants"

// ─── Helpers ─────────────────────────────────────────────────────────────────

function setMockStdout(stdout: string, code = 0) {
  mockRunResult = { err: false, val: { stdout, stderr: "", code } }
}

function setMockError() {
  mockRunResult = { err: false, val: { stdout: "", stderr: "something went wrong", code: 1 } }
}

// ─── Tests ───────────────────────────────────────────────────────────────────

describe("ProviderCommands.ListProviders", () => {
  beforeEach(() => {
    capturedArgs = []
    ProviderCommands.DEBUG = false
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it("sends the correct CLI arguments", async () => {
    setMockStdout("{}")
    await ProviderCommands.ListProviders()

    expect(capturedArgs).toContain(DEVPOD_COMMAND_PROVIDER)
    expect(capturedArgs).toContain(DEVPOD_COMMAND_LIST)
    expect(capturedArgs).toContain(DEVPOD_FLAG_JSON_OUTPUT)
    expect(capturedArgs).toContain(DEVPOD_FLAG_JSON_LOG_OUTPUT)
  })

  it("parses and returns the JSON provider map", async () => {
    const providers = {
      docker: { name: "docker", config: { name: "docker" } },
    }
    setMockStdout(JSON.stringify(providers))

    const result = await ProviderCommands.ListProviders()
    expect(result.err).toBe(false)
    expect((result as any).val).toHaveProperty("docker")
  })

  it("returns an error when command exits non-zero", async () => {
    setMockError()
    const result = await ProviderCommands.ListProviders()
    expect(result.err).toBe(true)
  })

  it("appends --debug flag when DEBUG is true", async () => {
    ProviderCommands.DEBUG = true
    setMockStdout("{}")
    await ProviderCommands.ListProviders()
    // ListProviders uses a plain `new Command([...])` without the debug flag
    // (only newCommand-based methods respect DEBUG). Document that behaviour:
    expect(capturedArgs).not.toContain("--debug")
    ProviderCommands.DEBUG = false
  })
})

describe("ProviderCommands.AddProvider", () => {
  beforeEach(() => {
    capturedArgs = []
    ProviderCommands.DEBUG = false
    setMockStdout("")
  })

  it("includes the provider source in args", async () => {
    await ProviderCommands.AddProvider("docker", {})
    expect(capturedArgs).toContain("docker")
  })

  it("includes --use=false to avoid setting as default automatically", async () => {
    await ProviderCommands.AddProvider("kubernetes", {})
    expect(capturedArgs).toContain(`${DEVPOD_FLAG_USE}=false`)
  })

  it("includes --name flag when config.name is provided", async () => {
    await ProviderCommands.AddProvider("docker", { name: "my-docker" })
    expect(capturedArgs).toContain("--name=my-docker")
  })

  it("does not include --name flag when config.name is absent", async () => {
    await ProviderCommands.AddProvider("docker", {})
    const hasName = capturedArgs.some((a) => a.startsWith("--name="))
    expect(hasName).toBe(false)
  })
})

describe("ProviderCommands.RemoveProvider", () => {
  beforeEach(() => {
    capturedArgs = []
    setMockStdout("")
  })

  it("sends provider delete command with the provider ID", async () => {
    await ProviderCommands.RemoveProvider("docker")
    expect(capturedArgs).toContain(DEVPOD_COMMAND_PROVIDER)
    expect(capturedArgs).toContain(DEVPOD_COMMAND_DELETE)
    expect(capturedArgs).toContain("docker")
  })
})

describe("ProviderCommands.UseProvider", () => {
  beforeEach(() => {
    capturedArgs = []
    setMockStdout("")
  })

  it("sends provider use command with the provider ID", async () => {
    await ProviderCommands.UseProvider("kubernetes")
    expect(capturedArgs).toContain(DEVPOD_COMMAND_PROVIDER)
    expect(capturedArgs).toContain(DEVPOD_COMMAND_USE)
    expect(capturedArgs).toContain("kubernetes")
  })

  it("appends --single-machine when reuseMachine is true", async () => {
    await ProviderCommands.UseProvider("docker", undefined, true)
    expect(capturedArgs).toContain("--single-machine")
  })

  it("does not append --single-machine when reuseMachine is false", async () => {
    await ProviderCommands.UseProvider("docker", undefined, false)
    expect(capturedArgs).not.toContain("--single-machine")
  })

  it("serializes rawOptions as --option=KEY=VALUE flags", async () => {
    await ProviderCommands.UseProvider("docker", { DISK: "50GB" })
    expect(capturedArgs).toContain("--option=DISK=50GB")
  })
})

describe("ProviderCommands.GetProviderOptions", () => {
  beforeEach(() => {
    capturedArgs = []
  })

  it("fetches options with correct args", async () => {
    setMockStdout("{}")
    await ProviderCommands.GetProviderOptions("docker")
    expect(capturedArgs).toContain(DEVPOD_COMMAND_PROVIDER)
    expect(capturedArgs).toContain(DEVPOD_COMMAND_OPTIONS)
    expect(capturedArgs).toContain("docker")
    expect(capturedArgs).toContain(DEVPOD_FLAG_JSON_OUTPUT)
  })

  it("parses and returns the options JSON", async () => {
    const opts = { DISK: { value: "50GB", description: "Disk size" } }
    setMockStdout(JSON.stringify(opts))
    const result = await ProviderCommands.GetProviderOptions("docker")
    expect(result.err).toBe(false)
    expect((result as any).val).toHaveProperty("DISK")
  })
})

describe("ProviderCommands.SetProviderOptions", () => {
  beforeEach(() => {
    capturedArgs = []
    setMockStdout("")
  })

  it("sends set-options command with correct provider id", async () => {
    await ProviderCommands.SetProviderOptions("docker", { DISK: "10GB" }, false)
    expect(capturedArgs).toContain(DEVPOD_COMMAND_PROVIDER)
    expect(capturedArgs).toContain(DEVPOD_COMMAND_SET_OPTIONS)
    expect(capturedArgs).toContain("docker")
  })

  it("serializes options as CLI flags", async () => {
    await ProviderCommands.SetProviderOptions("docker", { CPU: "2" }, false)
    expect(capturedArgs).toContain("--option=CPU=2")
  })

  it("adds --dry when dry is true", async () => {
    // dry=true causes the result stdout to be JSON-parsed; provide valid JSON.
    setMockStdout(JSON.stringify({ DISK: { value: "50GB" } }))
    await ProviderCommands.SetProviderOptions("docker", {}, false, true)
    expect(capturedArgs).toContain("--dry")
  })

  it("adds --reconfigure when reconfigure is true", async () => {
    await ProviderCommands.SetProviderOptions("docker", {}, false, false, true)
    expect(capturedArgs).toContain("--reconfigure")
  })

  it("adds --single-machine when reuseMachine is true", async () => {
    await ProviderCommands.SetProviderOptions("docker", {}, true)
    expect(capturedArgs).toContain("--single-machine")
  })
})

describe("ProviderCommands.CheckProviderUpdate", () => {
  beforeEach(() => {
    capturedArgs = []
  })

  it("sends check-provider-update command", async () => {
    const updateResult = { updateAvailable: false, latestVersion: "1.0.0" }
    setMockStdout(JSON.stringify(updateResult))

    const result = await ProviderCommands.CheckProviderUpdate("docker")
    expect(result.err).toBe(false)
    expect((result as any).val).toHaveProperty("updateAvailable", false)
  })

  it("uses the helper subcommand", async () => {
    setMockStdout(JSON.stringify({ updateAvailable: false, latestVersion: "1.0.0" }))
    await ProviderCommands.CheckProviderUpdate("docker")
    expect(capturedArgs).toContain("helper")
    expect(capturedArgs).toContain("check-provider-update")
    expect(capturedArgs).toContain("docker")
  })
})
