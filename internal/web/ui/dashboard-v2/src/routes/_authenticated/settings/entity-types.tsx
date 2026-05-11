import { createFileRoute } from '@tanstack/react-router'
import { SettingsEntityTypes } from '@/features/settings/entity-types'

export const Route = createFileRoute('/_authenticated/settings/entity-types')({
  component: SettingsEntityTypes,
})
