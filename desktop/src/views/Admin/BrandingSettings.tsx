import {
  Box,
  Button,
  FormControl,
  FormHelperText,
  FormLabel,
  HStack,
  Heading,
  Input,
  VStack,
  useToast,
} from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { useAuth, useBranding } from "@/contexts"
import { adminUpdateBranding, adminResetBranding } from "@/client/adminClient"
import type { BrandingSettings } from "@/client/authClient"

export function BrandingSettingsPanel() {
  const { token } = useAuth()
  const { branding, refresh } = useBranding()
  const toast = useToast()
  const [form, setForm] = useState<BrandingSettings>(branding)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setForm(branding)
  }, [branding])

  const set = (key: keyof BrandingSettings) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setForm((prev) => ({ ...prev, [key]: e.target.value }))
  }

  const handleSave = async () => {
    if (!token) return
    setSaving(true)
    try {
      await adminUpdateBranding(token, form)
      toast({ status: "success", title: "Branding updated" })
      refresh()
    } catch (e: unknown) {
      toast({ status: "error", title: "Save failed", description: (e as Error).message })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = async () => {
    if (!token) return
    if (!window.confirm("Reset all branding to defaults?")) return
    setSaving(true)
    try {
      await adminResetBranding(token)
      toast({ status: "success", title: "Branding reset to defaults" })
      refresh()
    } catch (e: unknown) {
      toast({ status: "error", title: "Reset failed", description: (e as Error).message })
    } finally {
      setSaving(false)
    }
  }

  return (
    <Box>
      <HStack justify="space-between" mb={4}>
        <Heading size="md">Branding</Heading>
      </HStack>

      <VStack spacing={4} align="stretch" maxW="500px">
        <FormControl>
          <FormLabel>App Name</FormLabel>
          <Input value={form.appName} onChange={set("appName")} placeholder="DevPod" />
          <FormHelperText>Shown in the title bar and login page</FormHelperText>
        </FormControl>

        <FormControl>
          <FormLabel>Logo URL</FormLabel>
          <Input value={form.logoUrl} onChange={set("logoUrl")} placeholder="https://…/logo.svg" />
          <FormHelperText>Custom logo displayed in the sidebar</FormHelperText>
        </FormControl>

        <FormControl>
          <FormLabel>Favicon URL</FormLabel>
          <Input value={form.faviconUrl} onChange={set("faviconUrl")} placeholder="https://…/favicon.ico" />
        </FormControl>

        <FormControl>
          <FormLabel>Provider Download URL</FormLabel>
          <Input
            value={form.providerDownloadUrl}
            onChange={set("providerDownloadUrl")}
            placeholder="https://github.com/loft-sh/devpod/releases"
          />
          <FormHelperText>URL shown on the Providers page for downloading providers</FormHelperText>
        </FormControl>

        <FormControl>
          <FormLabel>Docs URL</FormLabel>
          <Input value={form.docsUrl} onChange={set("docsUrl")} placeholder="https://devpod.sh/docs" />
        </FormControl>

        <FormControl>
          <FormLabel>Support URL</FormLabel>
          <Input value={form.supportUrl} onChange={set("supportUrl")} placeholder="https://…/support" />
        </FormControl>

        <FormControl>
          <FormLabel>Primary Color</FormLabel>
          <Input value={form.primaryColor} onChange={set("primaryColor")} placeholder="#4B6BFB" />
          <FormHelperText>CSS color value applied as --devpod-primary-color</FormHelperText>
        </FormControl>

        <HStack pt={2}>
          <Button colorScheme="blue" isLoading={saving} onClick={handleSave}>
            Save Changes
          </Button>
          <Button variant="ghost" colorScheme="red" isLoading={saving} onClick={handleReset}>
            Reset to Defaults
          </Button>
        </HStack>
      </VStack>
    </Box>
  )
}
