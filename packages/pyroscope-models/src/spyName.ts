import { z } from 'zod';

export const SpyNameSchema = z
  .enum([
    'dotnetspy',
    'ebpfspy',
    'gospy',
    'phpspy',
    'pyspy',
    'rbspy',
    'nodespy',
    'javaspy',
    'pyroscope-rs',

    'scrape', // for compability purposes, it should be golang
    'tracing',
    'unknown',
  ])
  .default('unknown');

export type SpyName = z.infer<typeof SpyNameSchema>;
