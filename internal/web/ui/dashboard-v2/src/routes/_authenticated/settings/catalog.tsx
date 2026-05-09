import { createFileRoute } from '@tanstack/react-router'
import { SettingsCatalog } from '@/features/settings/catalog'

export const Route = createFileRoute('/_authenticated/settings/catalog')({
  component: SettingsCatalog,
})
