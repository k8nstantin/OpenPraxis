import { z } from 'zod';

export const manifestCreateSchema = z.object({
  title: z.string().min(1, 'Title is required').max(200),
  description: z.string().max(50_000).optional(),
  status: z.enum(['open', 'in_progress', 'closed']).default('open'),
  project_id: z.string().uuid().optional(),
  depends_on: z.string().uuid().optional(),
});

export type ManifestCreateInput = z.infer<typeof manifestCreateSchema>;

export const manifestUpdateSchema = manifestCreateSchema.partial();
export type ManifestUpdateInput = z.infer<typeof manifestUpdateSchema>;
