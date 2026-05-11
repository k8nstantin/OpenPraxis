import { useQuery } from '@tanstack/react-query'

export interface EntityType {
  type_uid: string
  name: string
  display_name: string
  description: string
  color: string
  icon: string
}

export function useEntityTypes() {
  return useQuery({
    queryKey: ['entity-types'],
    queryFn: () =>
      fetch('/api/entity-types')
        .then((r) => r.json())
        .then((d) => (d.types ?? []) as EntityType[]),
    // 60 s stale time; types are invalidated on create via add-entity-dialog mutation.
    staleTime: 60_000,
  })
}
