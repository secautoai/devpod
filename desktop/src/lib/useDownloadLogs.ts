import { client } from "@/client/client"
import { TActionID } from "@/contexts"
import { useToast } from "@chakra-ui/react"
import { useMutation } from "@tanstack/react-query"
import * as dialog from "@tauri-apps/plugin-dialog"
import { IS_TAURI } from "./platform"
import { webActionStore } from "@/client/webClient/actionStore"

export function useDownloadLogs() {
  const toast = useToast()
  const { mutate, isLoading: isDownloading } = useMutation({
    mutationFn: async ({ actionID }: { actionID: TActionID }) => {
      if (!IS_TAURI) {
        // In web mode, build a text blob from the in-memory action log and
        // trigger a browser download.
        const events = webActionStore.read(actionID)
        const content = events.join("\n")
        const blob = new Blob([content], { type: "text/plain" })
        const url = URL.createObjectURL(blob)
        const a = document.createElement("a")
        a.href = url
        a.download = `devpod-action-${actionID}.log`
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
        URL.revokeObjectURL(url)
        return
      }

      const actionLogFile = (await client.workspaces.getActionLogFile(actionID)).unwrap()

      if (actionLogFile === undefined) {
        throw new Error(`Unable to retrieve file for action ${actionID}`)
      }

      const targetFile = await dialog.save({
        title: "Save Logs",
        filters: [{ name: "format", extensions: ["log", "txt"] }],
      })

      // user cancelled "save file" dialog
      if (targetFile === null) {
        return
      }

      await client.copyFile(actionLogFile, targetFile)
      client.open(targetFile)
    },
    onError(error) {
      toast({
        title: `Failed to save logs: ${error}`,
        status: "error",
        isClosable: true,
        duration: 30_000, // 30 sec
      })
    },
  })

  return { download: mutate, isDownloading }
}
