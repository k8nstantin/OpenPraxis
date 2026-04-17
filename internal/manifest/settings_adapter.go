package manifest

import (
	"context"
	"fmt"

	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// SettingsAdapter implements settings.ManifestLookup for the settings resolver.
// Like its task counterpart it lives outside internal/settings to preserve the
// import-cycle boundary — the resolver only references the minimal interface
// declared in internal/settings/resolver.go.
type SettingsAdapter struct {
	Store *Store
}

// GetManifestForSettings returns the product id for a manifest, or an error
// if the manifest cannot be found. The resolver uses this to promote a
// manifest-scoped lookup to its parent product scope during inheritance walks.
func (a *SettingsAdapter) GetManifestForSettings(_ context.Context, manifestID string) (settings.ManifestRec, error) {
	if a == nil || a.Store == nil {
		return settings.ManifestRec{}, fmt.Errorf("manifest settings adapter: store is nil")
	}
	m, err := a.Store.Get(manifestID)
	if err != nil {
		return settings.ManifestRec{}, fmt.Errorf("manifest settings adapter: get %q: %w", manifestID, err)
	}
	if m == nil {
		return settings.ManifestRec{}, fmt.Errorf("manifest settings adapter: manifest %q not found", manifestID)
	}
	return settings.ManifestRec{ID: m.ID, ProductID: m.ProjectID}, nil
}
