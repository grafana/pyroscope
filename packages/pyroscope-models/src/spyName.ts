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
];

export const SpyNameOther = [
  'scrape', // for compability purposes, it should be golang
  'tracing',
  'unknown',
];
type EnumParam = Parameters<typeof z.enum>[0];
export const SpyNameSchema = z
  .enum(SpyNameFirstClass.concat(SpyNameOther) as EnumParam)
  .default('unknown');

export type SpyName = z.infer<typeof SpyNameSchema>;
