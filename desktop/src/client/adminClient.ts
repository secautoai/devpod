import { WEB_SERVER_URL } from "@/lib/platform"
import type { UserPublic, BrandingSettings } from "./authClient"

export type OperationLog = {
  id: string
  username: string
  action: string
  target: string
  args: string[]
  startedAt: string
  finishedAt?: string
  exitCode: number
  error: string
}

export type ProviderPermissions = {
  unrestricted: boolean
  providers: string[] | null
}

// ─── Users ────────────────────────────────────────────────────────────────────

export async function adminListUsers(token: string): Promise<UserPublic[]> {
  const res = await fetch(`${WEB_SERVER_URL}/api/admin/users`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminCreateUser(
  token: string,
  username: string,
  password: string,
  role: "admin" | "user"
): Promise<UserPublic> {
  const res = await fetch(`${WEB_SERVER_URL}/api/admin/users`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify({ username, password, role }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminUpdateUser(
  token: string,
  username: string,
  updates: { role?: "admin" | "user"; dockerHost?: string }
): Promise<UserPublic> {
  const res = await fetch(`${WEB_SERVER_URL}/api/admin/users/${encodeURIComponent(username)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify(updates),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminDeleteUser(token: string, username: string): Promise<void> {
  const res = await fetch(`${WEB_SERVER_URL}/api/admin/users/${encodeURIComponent(username)}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error(await res.text())
}

// ─── Provider permissions ─────────────────────────────────────────────────────

export async function adminGetProviderPermissions(
  token: string,
  username: string
): Promise<ProviderPermissions> {
  const res = await fetch(
    `${WEB_SERVER_URL}/api/admin/users/${encodeURIComponent(username)}/providers`,
    { headers: { Authorization: `Bearer ${token}` } }
  )
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminSetProviderPermissions(
  token: string,
  username: string,
  providers: string[] | null
): Promise<void> {
  const res = await fetch(
    `${WEB_SERVER_URL}/api/admin/users/${encodeURIComponent(username)}/providers`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
      body: JSON.stringify({ providers }),
    }
  )
  if (!res.ok) throw new Error(await res.text())
}

// ─── Operation logs ───────────────────────────────────────────────────────────

export async function adminListLogs(
  token: string,
  opts: { username?: string; action?: string; limit?: number; offset?: number } = {}
): Promise<OperationLog[]> {
  const params = new URLSearchParams()
  if (opts.username) params.set("username", opts.username)
  if (opts.action) params.set("action", opts.action)
  if (opts.limit != null) params.set("limit", String(opts.limit))
  if (opts.offset != null) params.set("offset", String(opts.offset))
  const res = await fetch(`${WEB_SERVER_URL}/api/admin/logs?${params}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

// ─── Branding ─────────────────────────────────────────────────────────────────

export async function adminUpdateBranding(
  token: string,
  partial: Partial<BrandingSettings>
): Promise<BrandingSettings> {
  const res = await fetch(`${WEB_SERVER_URL}/api/admin/branding`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify(partial),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminResetBranding(token: string): Promise<BrandingSettings> {
  const res = await fetch(`${WEB_SERVER_URL}/api/admin/branding`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
