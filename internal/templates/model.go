package templates

// Scope values for prompt_templates.scope.
const (
	ScopeSystem   = "system"
	ScopeProduct  = "product"
	ScopeManifest = "manifest"
	ScopeTask     = "task"
	ScopeAgent    = "agent"
)

// Sections are the ordered prompt segments assembled by the runner.
var Sections = []string{
	SectionPreamble,
	SectionVisceralRules,
	SectionManifestSpec,
	SectionPriorContext,
	SectionTask,
	SectionInstructions,
	SectionGitWorkflow,
	SectionClosingProtocol,
}

const (
	SectionPreamble        = "preamble"
	SectionVisceralRules   = "visceral_rules"
	SectionManifestSpec    = "manifest_spec"
	SectionPriorContext    = "prior_context"
	SectionTask            = "task"
	SectionInstructions    = "instructions"
	SectionGitWorkflow     = "git_workflow"
	SectionClosingProtocol = "closing_protocol"
)

// Template mirrors a single row of prompt_templates. SCD-Type-2 audit
// columns (valid_from / valid_to / changed_by / reason) travel with every
// row so point-in-time reads and history queries can be served from one
// table.
type Template struct {
	ID          int64  `json:"id"`
	TemplateUID string `json:"template_uid"`
	Title       string `json:"title"`
	Scope       string `json:"scope"`
	ScopeID     string `json:"scope_id"`
	Section     string `json:"section"`
	Body        string `json:"body"`
	Status      string `json:"status"`
	Tags        string `json:"tags"`
	SourceNode  string `json:"source_node"`
	ValidFrom   string `json:"valid_from"`
	ValidTo     string `json:"valid_to"`
	ChangedBy   string `json:"changed_by"`
	Reason      string `json:"reason"`
	CreatedAt   string `json:"created_at"`
	DeletedAt   string `json:"deleted_at"`
}
