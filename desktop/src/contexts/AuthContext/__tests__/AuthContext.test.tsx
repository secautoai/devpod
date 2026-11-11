/**
 * Tests for AuthContext.
 *
 * Uses vitest + testing-library. IS_TAURI is mocked to false so we exercise
 * the web-mode auth flow (token stored in sessionStorage, apiMe on mount…).
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { render, screen, act, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { ReactNode } from "react"

// ── mock IS_TAURI = false ─────────────────────────────────────────────────────
vi.mock("@/lib/platform", () => ({
  IS_TAURI: false,
  WEB_SERVER_URL: "http://localhost",
  WEB_SERVER_WS_URL: "ws://localhost/api/command/stream",
}))

// ── mock WebCommand ───────────────────────────────────────────────────────────
vi.mock("@/client/webClient/command", () => ({
  WebCommand: { token: null },
}))

import { AuthProvider, useAuth } from "../AuthContext"
import * as authClient from "@/client/authClient"

// ── helpers ───────────────────────────────────────────────────────────────────

function TestConsumer() {
  const { token, user, isLoading, login, logout } = useAuth()
  return (
    <div>
      <div data-testid="loading">{String(isLoading)}</div>
      <div data-testid="token">{token ?? "null"}</div>
      <div data-testid="username">{user?.username ?? "null"}</div>
      <div data-testid="role">{user?.role ?? "null"}</div>
      {/* Swallow the error so it doesn't become an unhandled rejection in tests */}
      <button onClick={() => login("alice", "pass").catch(() => undefined)}>login</button>
      <button onClick={() => logout()}>logout</button>
    </div>
  )
}

function wrapper({ children }: { children: ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>
}

// ── setup / teardown ──────────────────────────────────────────────────────────

beforeEach(() => {
  sessionStorage.clear()
  vi.restoreAllMocks()
})

afterEach(() => {
  sessionStorage.clear()
  vi.restoreAllMocks()
})

// ── tests ─────────────────────────────────────────────────────────────────────

describe("AuthProvider – no stored token", () => {
  it("starts with null token, no loading", () => {
    render(<TestConsumer />, { wrapper })
    expect(screen.getByTestId("loading").textContent).toBe("false")
    expect(screen.getByTestId("token").textContent).toBe("null")
    expect(screen.getByTestId("username").textContent).toBe("null")
  })
})

describe("AuthProvider – login", () => {
  it("sets token and user after successful login", async () => {
    vi.spyOn(authClient, "apiLogin").mockResolvedValue({
      token: "tok-abc",
      user: { id: "1", username: "alice", role: "user", createdAt: "2025-01-01" },
    })

    render(<TestConsumer />, { wrapper })
    await userEvent.click(screen.getByText("login"))

    await waitFor(() => {
      expect(screen.getByTestId("token").textContent).toBe("tok-abc")
      expect(screen.getByTestId("username").textContent).toBe("alice")
      expect(screen.getByTestId("role").textContent).toBe("user")
    })
    expect(sessionStorage.getItem("devpod_web_token")).toBe("tok-abc")
  })

  it("does not update state when login throws", async () => {
    vi.spyOn(authClient, "apiLogin").mockRejectedValue(new Error("bad credentials"))

    render(<TestConsumer />, { wrapper })
    await act(async () => {
      await userEvent.click(screen.getByText("login")).catch(() => {
        // expected rejection
      })
    })

    // State should still be null
    expect(screen.getByTestId("token").textContent).toBe("null")
  })
})

describe("AuthProvider – logout", () => {
  it("clears token and user", async () => {
    vi.spyOn(authClient, "apiLogin").mockResolvedValue({
      token: "tok-xyz",
      user: { id: "2", username: "bob", role: "admin", createdAt: "2025-01-01" },
    })

    render(<TestConsumer />, { wrapper })

    await userEvent.click(screen.getByText("login"))
    await waitFor(() => expect(screen.getByTestId("token").textContent).toBe("tok-xyz"))

    await userEvent.click(screen.getByText("logout"))
    await waitFor(() => {
      expect(screen.getByTestId("token").textContent).toBe("null")
      expect(screen.getByTestId("username").textContent).toBe("null")
    })
    expect(sessionStorage.getItem("devpod_web_token")).toBeNull()
  })
})

describe("AuthProvider – restore from sessionStorage", () => {
  it("calls apiMe on mount if token is already stored", async () => {
    sessionStorage.setItem("devpod_web_token", "stored-tok")

    const mockMe = vi.spyOn(authClient, "apiMe").mockResolvedValue({
      id: "3",
      username: "carol",
      role: "admin",
      createdAt: "2025-01-01",
    })

    render(<TestConsumer />, { wrapper })

    // Initially loading
    expect(screen.getByTestId("loading").textContent).toBe("true")

    await waitFor(() => {
      expect(screen.getByTestId("loading").textContent).toBe("false")
      expect(screen.getByTestId("username").textContent).toBe("carol")
      expect(screen.getByTestId("token").textContent).toBe("stored-tok")
    })
    expect(mockMe).toHaveBeenCalledWith("stored-tok")
  })

  it("clears token when apiMe rejects (e.g. expired token)", async () => {
    sessionStorage.setItem("devpod_web_token", "expired-tok")
    vi.spyOn(authClient, "apiMe").mockRejectedValue(new Error("Unauthorized"))

    render(<TestConsumer />, { wrapper })

    await waitFor(() => {
      expect(screen.getByTestId("loading").textContent).toBe("false")
      expect(screen.getByTestId("token").textContent).toBe("null")
    })
    expect(sessionStorage.getItem("devpod_web_token")).toBeNull()
  })
})
