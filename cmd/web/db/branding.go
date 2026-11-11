package db

import (
	"encoding/json"
	"os"
	"sync"
)

// BrandingSettings holds admin-configurable UI customisation.
type BrandingSettings struct {
	AppName             string `json:"appName"`
	LogoURL             string `json:"logoUrl"`
	FaviconURL          string `json:"faviconUrl"`
	ProviderDownloadURL string `json:"providerDownloadUrl"`
	SupportURL          string `json:"supportUrl"`
	DocsURL             string `json:"docsUrl"`
	PrimaryColor        string `json:"primaryColor"`
}

// defaultBranding returns the built-in defaults.
func defaultBranding() BrandingSettings {
	return BrandingSettings{
		AppName:             "DevPod",
		LogoURL:             "",
		FaviconURL:          "",
		ProviderDownloadURL: "https://github.com/loft-sh/devpod/releases",
		SupportURL:          "",
		DocsURL:             "https://devpod.sh/docs",
		PrimaryColor:        "",
	}
}

// BrandingStore persists branding settings to a JSON file.
type BrandingStore struct {
	mu       sync.RWMutex
	settings BrandingSettings
	filePath string
}

func newBrandingStore(filePath string) (*BrandingStore, error) {
	s := &BrandingStore{
		settings: defaultBranding(),
		filePath: filePath,
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *BrandingStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	// Unmarshal into defaults so missing keys keep their default values
	settings := defaultBranding()
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}
	s.settings = settings
	return nil
}

func (s *BrandingStore) save() error {
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// Get returns the current branding settings.
func (s *BrandingStore) Get() BrandingSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// Update merges the provided partial settings (zero values are ignored).
func (s *BrandingStore) Update(partial BrandingSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if partial.AppName != "" {
		s.settings.AppName = partial.AppName
	}
	if partial.LogoURL != "" {
		s.settings.LogoURL = partial.LogoURL
	}
	if partial.FaviconURL != "" {
		s.settings.FaviconURL = partial.FaviconURL
	}
	if partial.ProviderDownloadURL != "" {
		s.settings.ProviderDownloadURL = partial.ProviderDownloadURL
	}
	if partial.SupportURL != "" {
		s.settings.SupportURL = partial.SupportURL
	}
	if partial.DocsURL != "" {
		s.settings.DocsURL = partial.DocsURL
	}
	if partial.PrimaryColor != "" {
		s.settings.PrimaryColor = partial.PrimaryColor
	}
	return s.save()
}

// Reset restores all settings to their default values.
func (s *BrandingStore) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = defaultBranding()
	return s.save()
}
