import { z } from 'zod';

export const AllProfilesSchema = z.record(
  z.object({
    // TODO(eh-am): in practice it's a UUID
    id: z.string(),
    name: z.string(),
    updatedAt: z.string(),
    // TODO(dogfrogfog): remove .optional() after BE implementation
    suffix: z.string().optional(),
  })
);

export type AllProfiles = z.infer<typeof AllProfilesSchema>;
