/**
 * In-browser action log store for web mode.
 *
 * The Tauri desktop app stores action logs in the Rust backend so they survive
 * component re-mounts.  In web mode we replicate that behaviour using a module-
 * level Map – it is shared across all React renders for the lifetime of the tab.
 */

const actionLogs = new Map<string, string[]>()

export const webActionStore = {
  write(actionId: string, data: string): void {
    const existing = actionLogs.get(actionId) ?? []
    existing.push(data)
    actionLogs.set(actionId, existing)
  },

  read(actionId: string): readonly string[] {
    return actionLogs.get(actionId) ?? []
  },

  /** Returns a synthetic "file path" string (not a real path in web mode). */
  filePath(actionId: string): string {
    return `web://action-logs/${actionId}`
  },

  clear(actionId: string): void {
    actionLogs.delete(actionId)
  },
}
