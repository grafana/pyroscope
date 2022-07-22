import { z } from 'zod';

export const SpyNameFirstClass = [
  'dotnetspy',
  'ebpfspy',
  'gospy',
  'phpspy',
  'pyspy',
  'rbspy',
  'nodespy',
  'javaspy',
  'pyroscope-rs',
] as const;

export const SpyNameOther = [
  'scrape', // for compability purposes, it should be golang
  'tracing',
  'unknown',
] as const;

const AllSpyNames = [...SpyNameFirstClass, ...SpyNameOther] as const;

export const SpyNameSchema = z.enum(AllSpyNames).optional().default('unknown');

export type SpyName = z.infer<typeof SpyNameSchema>;
