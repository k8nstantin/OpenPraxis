import { z } from 'zod';

export const productCreateSchema = z.object({
  title: z.string().min(1, 'Title is required').max(200),
  description: z.string().max(50_000).optional(),
  status: z.enum(['active', 'paused', 'archived']).default('active'),
  tags: z.array(z.string().min(1)).optional(),
  parent_id: z.string().uuid().optional(),
});

export type ProductCreateInput = z.infer<typeof productCreateSchema>;

export const productUpdateSchema = productCreateSchema.partial();
export type ProductUpdateInput = z.infer<typeof productUpdateSchema>;
