import { z } from 'zod';

export const taskCreateSchema = z.object({
  title: z.string().min(1, 'Title is required').max(200),
  description: z.string().max(50_000).optional(),
  manifest_id: z.string().uuid({ message: 'manifest_id must be a full UUID' }),
  depends_on: z.string().uuid().optional(),
  schedule_at: z.string().datetime().optional(),
});

export type TaskCreateInput = z.infer<typeof taskCreateSchema>;

export const taskUpdateSchema = taskCreateSchema.partial();
export type TaskUpdateInput = z.infer<typeof taskUpdateSchema>;
