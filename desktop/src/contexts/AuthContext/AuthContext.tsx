import { createContext, ReactNode, useCallback, useContext, useEffect, useState } from "react"
import { IS_TAURI } from "@/lib/platform"
import { apiLogin, apiMe, apiChangePassword, type UserPublic } from "@/client/authClient"
import { WebCommand } from "@/client/webClient/command"

const TOKEN_STORAGE_KEY = "devpod_web_token"

type AuthContextValue = {
  token: string | null
  user: UserPublic | null
  isLoading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: Readonly<{ children: ReactNode }>) {
  const [token, setToken] = useState<string | null>(() => {
    // Auth only applies in web mode; Tauri uses its own mechanisms
    if (IS_TAURI) return null
    return sessionStorage.getItem(TOKEN_STORAGE_KEY)
  })
  const [user, setUser] = useState<UserPublic | null>(null)
  const [isLoading, setIsLoading] = useState(!IS_TAURI && !!token)

  // On mount, restore user from stored token
  useEffect(() => {
    if (IS_TAURI || !token) {
      setIsLoading(false)
      return
    }
    apiMe(token)
      .then((u) => {
        setUser(u)
        WebCommand.token = token
      })
      .catch(() => {
        sessionStorage.removeItem(TOKEN_STORAGE_KEY)
        setToken(null)
        WebCommand.token = null
      })
      .finally(() => setIsLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const { token: tok, user: u } = await apiLogin(username, password)
    sessionStorage.setItem(TOKEN_STORAGE_KEY, tok)
    setToken(tok)
    setUser(u)
    WebCommand.token = tok
  }, [])

  const logout = useCallback(() => {
    sessionStorage.removeItem(TOKEN_STORAGE_KEY)
    setToken(null)
    setUser(null)
    WebCommand.token = null
  }, [])

  const changePassword = useCallback(
    async (currentPassword: string, newPassword: string) => {
      if (!token) throw new Error("Not authenticated")
      await apiChangePassword(token, currentPassword, newPassword)
    },
    [token]
  )

  return (
    <AuthContext.Provider value={{ token, user, isLoading, login, logout, changePassword }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
