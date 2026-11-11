import { createBrowserRouter } from "react-router-dom"
import { App, ErrorPage } from "./App"
import { ProRoot } from "./ProRoot"
import { Routes } from "./routes.constants"
import { Actions, Pro, Providers, Settings, Workspaces } from "./views"
import { LoginPage } from "./views/Login"
import { AdminLayout } from "./views/Admin"
import { ProtectedRoute } from "./components/ProtectedRoute"

export const router = createBrowserRouter([
  {
    path: Routes.LOGIN,
    element: <LoginPage />,
  },
  {
    path: Routes.ROOT,
    element: <App />,
    errorElement: <ErrorPage />,
    children: [
      {
        path: Routes.PRO,
        element: <ProRoot />,
        children: [
          {
            path: Routes.PRO_INSTANCE,
            element: <Pro.ProInstance />,
            children: [
              {
                index: true,
                element: <Pro.ListWorkspaces />,
              },
              {
                path: Routes.PRO_WORKSPACE,
                element: <Pro.Workspace />,
              },
              {
                path: Routes.PRO_WORKSPACE_CREATE,
                element: <Pro.CreateWorkspace />,
              },
              {
                path: Routes.PRO_WORKSPACE_SELECT_PRESET,
                element: <Pro.SelectPreset />,
              },
              { path: Routes.PRO_SETTINGS, element: <Pro.Settings /> },
              { path: Routes.PRO_CREDENTIALS, element: <Pro.Credentials /> },
              { path: Routes.PRO_PROFILE, element: <Pro.Profile /> },
            ],
          },
        ],
      },
      // All OSS routes require auth in web mode
      {
        element: <ProtectedRoute />,
        children: [
          {
            path: Routes.WORKSPACES,
            element: <Workspaces.Workspaces />,
            children: [
              {
                index: true,
                element: <Workspaces.ListWorkspaces />,
              },
              {
                path: Routes.WORKSPACE_CREATE,
                element: <Workspaces.CreateWorkspace />,
              },
            ],
          },
          {
            path: Routes.PROVIDERS,
            element: <Providers.Providers />,
            children: [
              { index: true, element: <Providers.ListProviders /> },
              {
                path: Routes.PROVIDER,
                element: <Providers.Provider />,
              },
            ],
          },
          {
            path: Routes.ACTIONS,
            element: <Actions.Actions />,
            children: [{ path: Routes.ACTION, element: <Actions.Action /> }],
          },
          { path: Routes.SETTINGS, element: <Settings.Settings /> },
          // Admin routes: require admin role
          {
            element: <ProtectedRoute requireAdmin />,
            children: [
              { path: Routes.ADMIN, element: <AdminLayout /> },
              { path: Routes.ADMIN_USERS, element: <AdminLayout /> },
              { path: Routes.ADMIN_LOGS, element: <AdminLayout /> },
              { path: Routes.ADMIN_BRANDING, element: <AdminLayout /> },
            ],
          },
        ],
      },
    ],
  },
])
