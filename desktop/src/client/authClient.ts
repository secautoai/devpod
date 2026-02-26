import { WEB_SERVER_URL } from "@/lib/platform"

export type UserPublic = {
  id: string
  username: string
  role: "admin" | "user"
  dockerHost?: string
  createdAt: string
}

export type LoginResponse = {
  token: string
  user: UserPublic
}

export type BrandingSettings = {
  appName: string
  logoUrl: string
  faviconUrl: string
  providerDownloadUrl: string
  supportUrl: string
  docsUrl: string
  primaryColor: string
}

export async function apiLogin(username: string, password: string): Promise<LoginResponse> {
  const res = await fetch(`${WEB_SERVER_URL}/api/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text.trim() || res.statusText)
  }
  return res.json()
}

export async function apiMe(token: string): Promise<UserPublic> {
  const res = await fetch(`${WEB_SERVER_URL}/api/me`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error("Unauthorized")
  }
  return res.json()
}

export async function apiChangePassword(
  token: string,
  currentPassword: string,
  newPassword: string
): Promise<void> {
  const res = await fetch(`${WEB_SERVER_URL}/api/user/password`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ currentPassword, newPassword }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text.trim() || res.statusText)
  }
}

export async function apiBranding(): Promise<BrandingSettings> {
  const res = await fetch(`${WEB_SERVER_URL}/api/branding`)
  if (!res.ok) {
    throw new Error("Failed to fetch branding")
  }
  return res.json()
}
