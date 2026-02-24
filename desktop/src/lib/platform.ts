export const isLinux = import.meta.env.TAURI_ENV_PLATFORM === "linux"
export const isMacOS = import.meta.env.TAURI_ENV_PLATFORM === "darwin"
export const isWindows = import.meta.env.TAURI_ENV_PLATFORM === "windows"

/**
 * True when the frontend is running inside a Tauri desktop window.
 * False when served by the `devpod web` HTTP server (browser context).
 */
export const IS_TAURI: boolean =
  typeof window !== "undefined" && "__TAURI__" in window

/**
 * Base URL for the `devpod web` HTTP API server.
 * Automatically derived from the page origin so it works on any host/port.
 */
export const WEB_SERVER_URL: string = IS_TAURI
  ? "http://localhost:25842" // keep backwards-compat with existing Tauri server
  : typeof window !== "undefined"
    ? window.location.origin
    : "http://localhost:8090"

/**
 * WebSocket URL for streaming command output in web mode.
 */
export const WEB_SERVER_WS_URL: string = WEB_SERVER_URL.replace(/^http/, "ws") + "/api/command/stream"
