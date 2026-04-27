import { z } from 'zod';

// Form-level zod schema for creating / updating a product. Stays loose
// where the API allows (description optional, tags optional) — strict
// fields are mirrored from the Go validation in product.Service.
export const productCreateSchema = z.object({
  title: z.string().trim().min(1, 'Title is required').max(200, 'Title is too long'),
  description: z.string().max(20000).optional(),
  tags: z.array(z.string().trim().min(1)).max(20).optional(),
  parent_id: z.string().uuid().optional(),
});
export type ProductCreateInput = z.infer<typeof productCreateSchema>;

export const productUpdateSchema = productCreateSchema.partial().extend({
  id: z.string().uuid(),
});
export type ProductUpdateInput = z.infer<typeof productUpdateSchema>;
