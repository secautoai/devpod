import react from "@vitejs/plugin-react"
import { resolve } from "node:path"
import { defineConfig } from "vite"
import devApiPlugin from "./vite-dev-api"

// When building for the web (`yarn build:web`), we produce a standard SPA bundle
// that can be embedded in the `devpod web` Go binary.
// The Tauri-specific multi-entry (main + updateWindow) is only used for desktop builds.
const isWebBuild = !process.env.TAURI_ENV_PLATFORM

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    react(),
    // In web-dev mode, serve API endpoints directly from the Vite dev server
    // so `yarn dev` works without needing the Go backend (`devpod web`).
    ...(isWebBuild ? [devApiPlugin()] : []),
  ],
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },

  // Vitest configuration – runs in jsdom so browser globals are available.
  // @ts-expect-error – vitest injects the `test` field at runtime; vite's own type doesn't know about it.
  test: {
    environment: "jsdom",
    globals: true,
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["node_modules", "src-tauri"],
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },

  // Vite options tailored for Tauri development and only applied in `tauri dev` or `tauri build`
  // prevent vite from obscuring rust errors
  clearScreen: false,
  // tauri expects a fixed port, fail if that port is not available
  server: {
    port: 1420,
    strictPort: !isWebBuild,
  },
  esbuild: {
    target: "safari14",
  },
  // to make use of `TAURI_DEBUG` and other env variables
  // https://tauri.studio/v1/api/config#buildconfig.beforedevcommand
  envPrefix: ["VITE_", "TAURI_"],
  build: {
    // Tauri supports es2021; for web mode use a more modern but widely-supported target
    target: process.env.TAURI_ENV_PLATFORM == "windows" ? "chrome105" : isWebBuild ? "es2020" : "safari13",
    // don't minify for debug builds
    minify: !process.env.TAURI_ENV_DEBUG ? "esbuild" : false,
    // produce sourcemaps for debug builds
    sourcemap: !!process.env.TAURI_ENV_DEBUG,
    rollupOptions: isWebBuild
      ? {
          // Web build: single entry point (SPA)
          input: {
            main: resolve(__dirname, "index.html"),
          },
        }
      : {
          // Desktop (Tauri) build: main window + updater window
          input: {
            main: resolve(__dirname, "index.html"),
            updateWindow: resolve(__dirname, "update-window/index.html"),
          },
        },
  },
})
