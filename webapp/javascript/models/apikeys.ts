import { z } from 'zod';

const zDateTime = z.string().transform((value) => Date.parse(value));

export const apikeyModel = z.object({
  id: z.number(),
  name: z.string(),
  role: z.string(),
  key: z.optional(z.string()),
  createdAt: zDateTime,
  expiresAt: z.optional(zDateTime),
});

export const apiKeysSchema = z.array(apikeyModel);

export type APIKeys = z.infer<typeof apiKeysSchema>;
export type APIKey = z.infer<typeof apikeyModel>;
