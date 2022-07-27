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

export const AllSpyNames = [...SpyNameFirstClass, ...SpyNameOther] as const;

export const SpyNameSchema = z.preprocess((val) => {
  if (!val) {
    return 'unknown';
  }
  return val;
}, z.enum(AllSpyNames).default('unknown'));

export type SpyName = z.infer<typeof SpyNameSchema>;
