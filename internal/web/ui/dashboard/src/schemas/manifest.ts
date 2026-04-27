import { z } from 'zod';

export const manifestStatusSchema = z.enum(['draft', 'open', 'closed', 'archive']);
export type ManifestStatus = z.infer<typeof manifestStatusSchema>;

export const manifestCreateSchema = z.object({
  title: z.string().trim().min(1, 'Title is required').max(200),
  description: z.string().max(50000).optional(),
  status: manifestStatusSchema.default('open'),
  product_id: z.string().uuid().optional(),
  depends_on: z.string().uuid().optional(),
});
export type ManifestCreateInput = z.infer<typeof manifestCreateSchema>;

export const manifestUpdateSchema = manifestCreateSchema.partial().extend({
  id: z.string().uuid(),
});
export type ManifestUpdateInput = z.infer<typeof manifestUpdateSchema>;
