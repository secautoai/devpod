/**
 * Unit tests for WebCommand.
 *
 * WebCommand.run() uses `fetch` and WebCommand.stream() uses `WebSocket`.
 * We stub both globals so tests run in jsdom without a real server.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { WebCommand } from "../command"
import { DEVPOD_UI_ENV_VAR } from "../../constants"

// ─── fetch stub ───────────────────────────────────────────────────────────────

function stubFetch(response: Record<string, unknown>, ok = true, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok,
      status,
      statusText: ok ? "OK" : "Server Error",
      json: () => Promise.resolve(response),
    } as Response)
  )
}

// ─── Setup / teardown ─────────────────────────────────────────────────────────

beforeEach(() => {
  // Reset static state between tests.
  WebCommand.ADDITIONAL_ENV_VARS = ""
  WebCommand.HTTP_PROXY = ""
  WebCommand.HTTPS_PROXY = ""
  WebCommand.NO_PROXY = ""
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

// ─── Constructor / env injection ─────────────────────────────────────────────

describe("WebCommand constructor", () => {
  it("always injects DEVPOD_UI_ENV_VAR=true", async () => {
    stubFetch({ stdout: "", stderr: "", code: 0 })
    const cmd = new WebCommand(["list"])
    // Trigger run so we can inspect what was sent.
    await cmd.run()

    const body = JSON.parse(
      (vi.mocked(fetch).mock.calls[0][1] as RequestInit).body as string
    )
    expect(body.env[DEVPOD_UI_ENV_VAR]).toBe("true")
  })

  it("injects HTTP_PROXY when static prop is set", async () => {
    WebCommand.HTTP_PROXY = "http://proxy.example.com"
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    await cmd.run()

    const body = JSON.parse(
      (vi.mocked(fetch).mock.calls[0][1] as RequestInit).body as string
    )
    expect(body.env.HTTP_PROXY).toBe("http://proxy.example.com")
  })

  it("injects HTTPS_PROXY when static prop is set", async () => {
    WebCommand.HTTPS_PROXY = "https://proxy.example.com"
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    await cmd.run()

    const body = JSON.parse(
      (vi.mocked(fetch).mock.calls[0][1] as RequestInit).body as string
    )
    expect(body.env.HTTPS_PROXY).toBe("https://proxy.example.com")
  })

  it("injects NO_PROXY when static prop is set", async () => {
    WebCommand.NO_PROXY = "localhost,127.0.0.1"
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    await cmd.run()

    const body = JSON.parse(
      (vi.mocked(fetch).mock.calls[0][1] as RequestInit).body as string
    )
    expect(body.env.NO_PROXY).toBe("localhost,127.0.0.1")
  })

  it("does not inject proxy vars when static props are empty", async () => {
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["version"])
    await cmd.run()

    const body = JSON.parse(
      (vi.mocked(fetch).mock.calls[0][1] as RequestInit).body as string
    )
    expect(body.env.HTTP_PROXY).toBeUndefined()
    expect(body.env.HTTPS_PROXY).toBeUndefined()
    expect(body.env.NO_PROXY).toBeUndefined()
  })

  it("parses ADDITIONAL_ENV_VARS CSV into individual env entries", async () => {
    WebCommand.ADDITIONAL_ENV_VARS = "MY_KEY=my_val,ANOTHER=yes"
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    await cmd.run()

    const body = JSON.parse(
      (vi.mocked(fetch).mock.calls[0][1] as RequestInit).body as string
    )
    expect(body.env.MY_KEY).toBe("my_val")
    expect(body.env.ANOTHER).toBe("yes")
  })

  it("ignores malformed pairs in ADDITIONAL_ENV_VARS", async () => {
    WebCommand.ADDITIONAL_ENV_VARS = "GOOD=value,BADENTRY,=nokey"
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    await cmd.run()

    const body = JSON.parse(
      (vi.mocked(fetch).mock.calls[0][1] as RequestInit).body as string
    )
    expect(body.env.GOOD).toBe("value")
    expect(body.env.BADENTRY).toBeUndefined()
  })
})

// ─── run() ────────────────────────────────────────────────────────────────────

describe("WebCommand.run", () => {
  it("sends a POST to /api/command with args", async () => {
    stubFetch({ stdout: '{"version":"1.0"}', stderr: "", code: 0 })

    const cmd = new WebCommand(["version", "--output=json"])
    await cmd.run()

    expect(fetch).toHaveBeenCalledOnce()
    const [url, opts] = vi.mocked(fetch).mock.calls[0]
    expect(String(url)).toContain("/api/command")
    expect((opts as RequestInit).method).toBe("POST")

    const body = JSON.parse((opts as RequestInit).body as string)
    expect(body.args).toEqual(["version", "--output=json"])
  })

  it("returns a successful Result on exit code 0", async () => {
    stubFetch({ stdout: "output", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    const result = await cmd.run()

    expect(result.err).toBe(false)
    expect((result as any).val.code).toBe(0)
  })

  it("returns a failure Result when fetch fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network error")))

    const cmd = new WebCommand(["list"])
    const result = await cmd.run()

    expect(result.err).toBe(true)
  })

  it("returns a failure Result on non-ok HTTP response", async () => {
    stubFetch({ message: "server down" }, false, 500)

    const cmd = new WebCommand(["list"])
    const result = await cmd.run()

    expect(result.err).toBe(true)
  })
})

// ─── Auth token injection ──────────────────────────────────────────────────────

describe("WebCommand auth token", () => {
  beforeEach(() => {
    WebCommand.token = null
  })

  afterEach(() => {
    WebCommand.token = null
  })

  it("attaches Authorization header when token is set", async () => {
    WebCommand.token = "test-jwt-token"
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    await cmd.run()

    const [, opts] = vi.mocked(fetch).mock.calls[0]
    const headers = (opts as RequestInit).headers as Record<string, string>
    expect(headers["Authorization"]).toBe("Bearer test-jwt-token")
  })

  it("omits Authorization header when token is null", async () => {
    WebCommand.token = null
    stubFetch({ stdout: "", stderr: "", code: 0 })

    const cmd = new WebCommand(["list"])
    await cmd.run()

    const [, opts] = vi.mocked(fetch).mock.calls[0]
    const headers = (opts as RequestInit).headers as Record<string, string>
    expect(headers["Authorization"]).toBeUndefined()
  })
})
