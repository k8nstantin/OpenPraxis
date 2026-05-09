import { createFileRoute } from '@tanstack/react-router'
import { SettingsScope } from '@/features/settings/scope'

export const Route = createFileRoute('/_authenticated/settings/scope')({
  component: SettingsScope,
})
