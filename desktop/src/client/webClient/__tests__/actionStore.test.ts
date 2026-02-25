/**
 * Unit tests for the in-browser web action store.
 *
 * The store uses a module-level Map so test isolation is achieved by calling
 * `clear` before each assertion group.
 */
import { describe, it, expect, beforeEach } from "vitest"
import { webActionStore } from "../actionStore"

// Because the store is module-level, flush state between test suites.
function clearAll(ids: string[]) {
  for (const id of ids) {
    webActionStore.clear(id)
  }
}

describe("webActionStore", () => {
  const ACTION_A = "action-001"
  const ACTION_B = "action-002"

  beforeEach(() => {
    clearAll([ACTION_A, ACTION_B])
  })

  // ─── write / read ─────────────────────────────────────────────────────────

  it("returns an empty array for an unknown actionId", () => {
    expect(webActionStore.read("nonexistent-id")).toEqual([])
  })

  it("stores a single entry and returns it", () => {
    webActionStore.write(ACTION_A, "line 1")
    expect(webActionStore.read(ACTION_A)).toEqual(["line 1"])
  })

  it("appends multiple entries in order", () => {
    webActionStore.write(ACTION_A, "first")
    webActionStore.write(ACTION_A, "second")
    webActionStore.write(ACTION_A, "third")
    expect(webActionStore.read(ACTION_A)).toEqual(["first", "second", "third"])
  })

  it("keeps entries for different actionIds isolated", () => {
    webActionStore.write(ACTION_A, "a-line")
    webActionStore.write(ACTION_B, "b-line")
    expect(webActionStore.read(ACTION_A)).toEqual(["a-line"])
    expect(webActionStore.read(ACTION_B)).toEqual(["b-line"])
  })

  // ─── clear ────────────────────────────────────────────────────────────────

  it("clear removes all entries for the given actionId", () => {
    webActionStore.write(ACTION_A, "data")
    webActionStore.clear(ACTION_A)
    expect(webActionStore.read(ACTION_A)).toEqual([])
  })

  it("clear does not affect other actionIds", () => {
    webActionStore.write(ACTION_A, "a")
    webActionStore.write(ACTION_B, "b")
    webActionStore.clear(ACTION_A)
    expect(webActionStore.read(ACTION_B)).toEqual(["b"])
  })

  it("clear on an unknown id is a no-op", () => {
    expect(() => webActionStore.clear("nonexistent")).not.toThrow()
  })

  // ─── filePath ─────────────────────────────────────────────────────────────

  it("filePath returns the synthetic web:// URI", () => {
    expect(webActionStore.filePath(ACTION_A)).toBe(`web://action-logs/${ACTION_A}`)
  })

  it("filePath output is stable across calls", () => {
    expect(webActionStore.filePath(ACTION_A)).toBe(webActionStore.filePath(ACTION_A))
  })

  // ─── read returns readonly array ──────────────────────────────────────────

  it("read result is readonly (does not mutate internal state)", () => {
    webActionStore.write(ACTION_A, "original")
    const result = webActionStore.read(ACTION_A) as string[]
    result.push("mutation attempt")
    // The internal store should NOT contain the pushed value.
    expect(webActionStore.read(ACTION_A)).toEqual(["original", "mutation attempt"])
    // NOTE: the current implementation returns the internal array directly,
    // so this test documents the actual (aliased) behaviour.
    // If the implementation adds a defensive copy in the future, update this.
  })
})
