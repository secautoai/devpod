/**
 * Tests for ProtectedRoute.
 *
 * Verifies redirect-to-login when unauthenticated, render when authenticated,
 * and admin-only enforcement.
 */
import { describe, it, expect, vi } from "vitest"
import { render, screen } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router-dom"
import { ReactNode } from "react"

// ── mock IS_TAURI = false ─────────────────────────────────────────────────────
vi.mock("@/lib/platform", () => ({
  IS_TAURI: false,
  WEB_SERVER_URL: "http://localhost",
  WEB_SERVER_WS_URL: "ws://localhost/api/command/stream",
}))

import { ProtectedRoute } from "../ProtectedRoute"

// ── Auth context mock ─────────────────────────────────────────────────────────

const mockAuthValue = {
  token: null as string | null,
  user: null as { username: string; role: "admin" | "user" } | null,
  isLoading: false,
  login: vi.fn(),
  logout: vi.fn(),
  changePassword: vi.fn(),
}

vi.mock("@/contexts", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/contexts")>()
  return {
    ...actual,
    useAuth: () => mockAuthValue,
  }
})

// ── helpers ───────────────────────────────────────────────────────────────────

function renderRoute(
  initialPath: string,
  requireAdmin = false,
  WrappedContent: ReactNode = <div>Protected content</div>
) {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/login" element={<div>Login page</div>} />
        <Route path="/workspaces" element={<div>Workspaces</div>} />
        <Route element={<ProtectedRoute requireAdmin={requireAdmin} />}>
          <Route path="/protected" element={WrappedContent} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

// ── tests ─────────────────────────────────────────────────────────────────────

describe("ProtectedRoute – unauthenticated", () => {
  it("redirects to /login when no token", () => {
    mockAuthValue.token = null
    mockAuthValue.user = null
    mockAuthValue.isLoading = false

    renderRoute("/protected")
    expect(screen.getByText("Login page")).toBeInTheDocument()
    expect(screen.queryByText("Protected content")).not.toBeInTheDocument()
  })
})

describe("ProtectedRoute – authenticated user", () => {
  it("renders children when token is set", () => {
    mockAuthValue.token = "valid-token"
    mockAuthValue.user = { username: "alice", role: "user" }
    mockAuthValue.isLoading = false

    renderRoute("/protected")
    expect(screen.getByText("Protected content")).toBeInTheDocument()
  })
})

describe("ProtectedRoute – admin required", () => {
  it("redirects to /workspaces for non-admin users", () => {
    mockAuthValue.token = "valid-token"
    mockAuthValue.user = { username: "alice", role: "user" }
    mockAuthValue.isLoading = false

    renderRoute("/protected", true)
    expect(screen.getByText("Workspaces")).toBeInTheDocument()
    expect(screen.queryByText("Protected content")).not.toBeInTheDocument()
  })

  it("renders children for admin users", () => {
    mockAuthValue.token = "admin-token"
    mockAuthValue.user = { username: "root", role: "admin" }
    mockAuthValue.isLoading = false

    renderRoute("/protected", true)
    expect(screen.getByText("Protected content")).toBeInTheDocument()
  })
})

describe("ProtectedRoute – loading state", () => {
  it("shows spinner while loading (no redirect)", () => {
    mockAuthValue.token = null
    mockAuthValue.user = null
    mockAuthValue.isLoading = true

    renderRoute("/protected")
    // Should not redirect, just show spinner
    expect(screen.queryByText("Login page")).not.toBeInTheDocument()
    expect(screen.queryByText("Protected content")).not.toBeInTheDocument()
  })
})
