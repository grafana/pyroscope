import { z } from 'zod';

export type SpyNameFirstClassType = (typeof SpyNameFirstClass)[number];

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
  'scrape', // for compatibility purposes, it should be golang
  'tracing',
  'unknown',
] as const;

export const AllSpyNames = [...SpyNameFirstClass, ...SpyNameOther] as const;

export const SpyNameSchema = z.preprocess((val) => {
  if (!val || !AllSpyNames.includes(val as (typeof AllSpyNames)[number])) {
    return 'unknown';
  }
  return val;
}, z.enum(AllSpyNames).default('unknown'));

export type SpyName = z.infer<typeof SpyNameSchema>;
