import { z } from 'zod';

export const taskStatusSchema = z.enum([
  'waiting',
  'scheduled',
  'running',
  'completed',
  'failed',
  'cancelled',
]);
export type TaskStatus = z.infer<typeof taskStatusSchema>;

export const taskCreateSchema = z.object({
  title: z.string().trim().min(1, 'Title is required').max(200),
  description: z.string().max(50000).optional(),
  manifest_id: z.string().uuid('Manifest is required'),
  depends_on: z.string().uuid().optional(),
});
export type TaskCreateInput = z.infer<typeof taskCreateSchema>;
