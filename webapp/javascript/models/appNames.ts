import { z } from 'zod';

export const appNamesModel = z.array(z.string());

export type AppNames = z.infer<typeof appNamesModel>;
