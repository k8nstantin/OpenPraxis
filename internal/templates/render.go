package templates

import (
	"fmt"
	"strings"
	"text/template"
	"time"
)

// TaskView is the minimal task payload a prompt template can reference.
// A view type (rather than *task.Task) keeps the templates package free of
// any dependency on internal/task, so the import arrow only points one way.
type TaskView struct {
	ID          string
	Title       string
	Description string
	Agent       string
}

// ManifestView is the minimal manifest payload surfaced to templates.
type ManifestView struct {
	ID      string
	Title   string
	Content string
}

// ProductView is the minimal product payload surfaced to templates.
type ProductView struct {
	ID    string
	Title string
}

// PromptData is the full shape a template body may address. Fields that
// aren't used by the current default bodies are still populated so
// future operator-authored templates (RC/M2..M6) can reference them
// without another refactor.
type PromptData struct {
	Task          TaskView
	Manifest      ManifestView
	Product       ProductView
	Settings      map[string]any
	PriorRuns     []string
	OtherComments []string
	VisceralRules string
	// BranchPrefix is the resolved branch_prefix knob value (e.g.
	// "openpraxis", or "qa" when overridden at product scope). The
	// default <git_workflow> template composes the full branch name
	// as "{{.BranchPrefix}}/{{.Task.ID}}".
	BranchPrefix string
	Now          time.Time
}

// Render parses `body` as a text/template and executes it against data.
// Parse errors are returned verbatim; execution errors wrap the section
// name when one is supplied.
func Render(body string, data PromptData) (string, error) {
	t, err := template.New("prompt").Parse(body)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return b.String(), nil
}
