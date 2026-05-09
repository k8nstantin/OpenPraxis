import { createFileRoute } from '@tanstack/react-router'
import { SettingsResolved } from '@/features/settings/resolved'

export const Route = createFileRoute('/_authenticated/settings/resolved')({
  component: SettingsResolved,
})
