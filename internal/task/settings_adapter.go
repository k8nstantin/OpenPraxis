package task

import (
	"context"
	"fmt"

	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// SettingsAdapter implements settings.TaskLookup for the settings resolver.
// It lives in internal/task (not internal/settings) so the settings package
// stays free of a task import — the resolver needs only the minimal interface
// declared in internal/settings/resolver.go.
type SettingsAdapter struct {
	Store *Store
}

// GetTaskForSettings returns the manifest id for a task, or an error if the
// task cannot be found. The resolver uses this to promote a task-scoped lookup
// to its parent manifest scope during inheritance walks.
func (a *SettingsAdapter) GetTaskForSettings(_ context.Context, taskID string) (settings.TaskRec, error) {
	if a == nil || a.Store == nil {
		return settings.TaskRec{}, fmt.Errorf("task settings adapter: store is nil")
	}
	t, err := a.Store.Get(taskID)
	if err != nil {
		return settings.TaskRec{}, fmt.Errorf("task settings adapter: get %q: %w", taskID, err)
	}
	if t == nil {
		return settings.TaskRec{}, fmt.Errorf("task settings adapter: task %q not found", taskID)
	}
	return settings.TaskRec{ID: t.ID, ManifestID: t.ManifestID}, nil
}
