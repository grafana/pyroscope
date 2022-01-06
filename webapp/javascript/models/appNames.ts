import { z } from 'zod';

export const appNamesModel = z.array(z.string());

export type ApPNames = z.infer<typeof appNamesModel>;
