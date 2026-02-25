/**
 * Unit tests for pure utility functions exported from command.ts.
 *
 * These functions are framework-agnostic and require no mocking.
 */
import { describe, it, expect } from "vitest"
import { isOk, toFlagArg, serializeRawOptions } from "../command"

// ─── isOk ─────────────────────────────────────────────────────────────────────

describe("isOk", () => {
  it("returns true when exit code is 0", () => {
    expect(isOk({ stdout: "", stderr: "", code: 0 })).toBe(true)
  })

  it("returns false when exit code is non-zero", () => {
    expect(isOk({ stdout: "", stderr: "", code: 1 })).toBe(false)
    expect(isOk({ stdout: "", stderr: "", code: 127 })).toBe(false)
  })

  it("returns false for negative exit codes", () => {
    expect(isOk({ stdout: "", stderr: "", code: -1 })).toBe(false)
  })
})

// ─── toFlagArg ───────────────────────────────────────────────────────────────

describe("toFlagArg", () => {
  it("joins flag and value with =", () => {
    expect(toFlagArg("--output", "json")).toBe("--output=json")
  })

  it("works with single-dash flags", () => {
    expect(toFlagArg("-v", "3")).toBe("-v=3")
  })

  it("handles empty value", () => {
    expect(toFlagArg("--debug", "")).toBe("--debug=")
  })

  it("handles value containing =", () => {
    expect(toFlagArg("--option", "KEY=VALUE")).toBe("--option=KEY=VALUE")
  })
})

// ─── serializeRawOptions ─────────────────────────────────────────────────────

describe("serializeRawOptions", () => {
  it("returns an empty array for empty options", () => {
    expect(serializeRawOptions({})).toEqual([])
  })

  it("formats each key-value pair as --option=KEY=VALUE", () => {
    const result = serializeRawOptions({ DISK: "50GB" })
    expect(result).toEqual(["--option=DISK=50GB"])
  })

  it("serializes multiple options", () => {
    const result = serializeRawOptions({ DISK: "50GB", CPU: "4" })
    expect(result).toHaveLength(2)
    expect(result).toContain("--option=DISK=50GB")
    expect(result).toContain("--option=CPU=4")
  })

  it("accepts a custom flag name", () => {
    const result = serializeRawOptions({ KEY: "val" }, "--provider-option")
    expect(result).toEqual(["--provider-option=KEY=val"])
  })

  it("coerces non-string values to strings", () => {
    // In practice the caller always passes strings, but the type allows unknown.
    const result = serializeRawOptions({ count: 3 as unknown as string })
    expect(result).toEqual(["--option=count=3"])
  })
})
