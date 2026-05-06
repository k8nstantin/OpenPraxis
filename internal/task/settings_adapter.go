package task

import (
	"context"

	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// SettingsAdapter implements settings.TaskLookup for the settings resolver.
// It lives in internal/task (not internal/settings) so the settings package
// stays free of a task import — the resolver needs only the minimal interface
// declared in internal/settings/resolver.go.
type SettingsAdapter struct {
	Store *Store
}

// GetTaskForSettings returns the manifest id for a task so the settings
// resolver can walk up the task → manifest → product → system hierarchy.
// For entity-only tasks (no legacy tasks row), returns the task with an
// empty ManifestID — the resolver falls through to system defaults.
func (a *SettingsAdapter) GetTaskForSettings(_ context.Context, taskID string) (settings.TaskRec, error) {
	if a == nil || a.Store == nil {
		return settings.TaskRec{ID: taskID}, nil
	}
	t, err := a.Store.Get(taskID)
	if err != nil || t == nil {
		// Entity-only task — no legacy row. Return the ID with no manifest
		// so the resolver uses system-level defaults.
		return settings.TaskRec{ID: taskID}, nil
	}
	return settings.TaskRec{ID: t.ID, ManifestID: t.ManifestID}, nil
}
