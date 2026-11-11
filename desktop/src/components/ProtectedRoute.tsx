import { Box, Spinner } from "@chakra-ui/react"
import { Navigate, Outlet } from "react-router-dom"
import { useAuth } from "@/contexts"
import { IS_TAURI } from "@/lib/platform"
import { Routes } from "@/routes.constants"

type ProtectedRouteProps = {
  requireAdmin?: boolean
}

/**
 * Wraps child routes so they require authentication (web mode only).
 * In Tauri desktop mode auth is not required and children are rendered directly.
 * When requireAdmin=true, also checks that the user has the admin role.
 */
export function ProtectedRoute({ requireAdmin = false }: ProtectedRouteProps) {
  const { token, user, isLoading } = useAuth()

  // Tauri mode: no auth gate
  if (IS_TAURI) return <Outlet />

  if (isLoading) {
    return (
      <Box
        height="100vh"
        display="flex"
        alignItems="center"
        justifyContent="center">
        <Spinner size="xl" />
      </Box>
    )
  }

  if (!token) {
    return <Navigate to={Routes.LOGIN} replace />
  }

  if (requireAdmin && user?.role !== "admin") {
    return <Navigate to={Routes.WORKSPACES} replace />
  }

  return <Outlet />
}
