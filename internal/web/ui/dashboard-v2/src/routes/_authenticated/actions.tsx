import { createFileRoute } from '@tanstack/react-router'
import { ActionsLogPage } from '@/features/actions'

// Actions Log — top-level audit surface. Replaces the legacy /active
// route (placeholder) per the product 019dd94e-99f scope: every tool
// call across every peer, agent, and session in one searchable
// timeline. Read-only; mutation paths (cancel, redact) live elsewhere.
export const Route = createFileRoute('/_authenticated/actions')({
  component: ActionsLogPage,
})
