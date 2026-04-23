package templates

// Default section bodies. These are the canonical seed values for the
// system-scope rows — reproducing the pre-RC/M1 hardcoded buildPrompt()
// output byte-for-byte when rendered with the equivalent PromptData.
//
// Ordering / separator notes: each body carries its own trailing
// whitespace, so the runner just concatenates the seven rendered
// sections with no extra joiner. The closing_protocol body folds in
// the "Report completion when done." tail since that line used to be
// appended after the final section wrapper.

const defaultPreamble = "You are executing a scheduled task for OpenPraxis.\n\n"

const defaultVisceralRules = `{{if .VisceralRules}}<visceral_rules>
MANDATORY — follow every rule without exception.

{{.VisceralRules}}
</visceral_rules>

{{end}}`

const defaultManifestSpec = `<manifest_spec title={{printf "%q" .Manifest.Title}}>
{{.Manifest.Content}}
</manifest_spec>

`

const defaultTask = `<task title={{printf "%q" .Task.Title}} id={{printf "%q" .Task.ID}}>
{{if .Task.Description}}{{.Task.Description}}
{{end}}</task>

`

const defaultInstructions = `<instructions>
Follow the manifest spec exactly. Work autonomously.
Call visceral_rules and visceral_confirm first.
</instructions>

`

const defaultGitWorkflow = `<git_workflow>
MANDATORY — every task gets its own branch and PR.

1. Before making ANY code changes, create a new branch:
   git checkout -b openpraxis/{{.BranchPrefix}}
2. Make all your changes on this branch.
3. Commit your work with a descriptive message.
4. Push the branch: git push -u origin openpraxis/{{.BranchPrefix}}
5. Create a pull request using: gh pr create --title "<title>" --body "<summary>"
6. Include the PR URL in your final output.

NEVER work on an existing branch. NEVER push to main.
</git_workflow>

`

const defaultClosingProtocol = `<closing_protocol>
MANDATORY — before your final commit+push, call the MCP tool:

    mcp__openpraxis__comment_add
      target_type = "task"
      target_id   = "{{.Task.ID}}"
      type        = "execution_review"
      author      = "agent"
      body        = <markdown summary>

The body should include:
- **What shipped** — files created/edited, key APIs, what's testable
- **Gates self-check** — which acceptance-criteria bullets you verified locally (git gate: commits exist; build gate: go build passes; manifest gate: deliverables addressed)
- **What the next task should expect** — APIs, error codes, file layout
- **Anything surprising** — bugs found, decisions taken, followups to file

This comment is your execution review — the canonical per-task home. The runner records an amnesia flag if this call is missing.
</closing_protocol>

Report completion when done.
`

// SystemDefaults returns the canonical default body for each section.
// Exported so tests and the seed can use the exact same strings.
func SystemDefaults() map[string]string {
	return map[string]string{
		SectionPreamble:        defaultPreamble,
		SectionVisceralRules:   defaultVisceralRules,
		SectionManifestSpec:    defaultManifestSpec,
		SectionTask:            defaultTask,
		SectionInstructions:    defaultInstructions,
		SectionGitWorkflow:     defaultGitWorkflow,
		SectionClosingProtocol: defaultClosingProtocol,
	}
}

// SystemDefaultTitles returns human-readable titles for each default.
func SystemDefaultTitles() map[string]string {
	return map[string]string{
		SectionPreamble:        "System preamble",
		SectionVisceralRules:   "Visceral rules block",
		SectionManifestSpec:    "Manifest spec block",
		SectionTask:            "Task block",
		SectionInstructions:    "Instructions block",
		SectionGitWorkflow:     "Git workflow block",
		SectionClosingProtocol: "Closing protocol + completion line",
	}
}
