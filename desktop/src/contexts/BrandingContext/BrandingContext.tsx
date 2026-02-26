import { createContext, ReactNode, useContext, useEffect, useState } from "react"
import { IS_TAURI } from "@/lib/platform"
import { apiBranding, type BrandingSettings } from "@/client/authClient"

const defaultBranding: BrandingSettings = {
  appName: "DevPod",
  logoUrl: "",
  faviconUrl: "",
  providerDownloadUrl: "https://github.com/loft-sh/devpod/releases",
  supportUrl: "",
  docsUrl: "https://devpod.sh/docs",
  primaryColor: "",
}

type BrandingContextValue = {
  branding: BrandingSettings
  refresh: () => void
}

const BrandingContext = createContext<BrandingContextValue>({
  branding: defaultBranding,
  refresh: () => undefined,
})

export function BrandingProvider({ children }: Readonly<{ children: ReactNode }>) {
  const [branding, setBranding] = useState<BrandingSettings>(defaultBranding)

  const fetchBranding = () => {
    if (IS_TAURI) return
    apiBranding()
      .then(setBranding)
      .catch(() => {
        /* use defaults on error */
      })
  }

  useEffect(() => {
    fetchBranding()
  }, [])

  // Apply branding side-effects
  useEffect(() => {
    if (!IS_TAURI) {
      if (branding.appName) {
        document.title = branding.appName
      }
      if (branding.faviconUrl) {
        let link = document.querySelector<HTMLLinkElement>("link[rel~='icon']")
        if (!link) {
          link = document.createElement("link")
          link.rel = "icon"
          document.head.appendChild(link)
        }
        link.href = branding.faviconUrl
      }
      if (branding.primaryColor) {
        document.documentElement.style.setProperty("--devpod-primary-color", branding.primaryColor)
      }
    }
  }, [branding])

  return (
    <BrandingContext.Provider value={{ branding, refresh: fetchBranding }}>
      {children}
    </BrandingContext.Provider>
  )
}

export function useBranding(): BrandingContextValue {
  return useContext(BrandingContext)
}
