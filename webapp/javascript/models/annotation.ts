import { z } from 'zod';

export const AnnotationSchema = z.object({
  content: z.string(),
  // TODO(eh-am): validate it's a valid unix timestamp
  timestamp: z.number(),
});

export type Annotation = z.infer<typeof AnnotationSchema>;
