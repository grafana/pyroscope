/* eslint-disable default-case, @typescript-eslint/switch-exhaustiveness-check, consistent-return */
import { Units } from '@pyroscope/models/src';
import { SpyName } from '@pyroscope/models/src/spyName';

export const humanizeSpyname = (n: SpyName) => {
  switch (n) {
    case 'gospy':
      return 'Go';
    case 'pyspy':
      return 'Python';
    case 'phpspy':
      return 'PHP';
    case 'pyroscope-rs':
      return 'Rust';
    case 'dotnetspy':
      return '.NET';
    case 'ebpfspy':
      return 'eBPF';
    case 'rbspy':
      return 'Ruby';
    case 'nodespy':
      return 'NodeJS';
    case 'javaspy':
      return 'Java';
  }
};

export const humanizeUnits = (u: Units) => {
  switch (u) {
    case 'samples':
      return 'Samples';
    case 'objects':
      return 'Objects';
    case 'goroutines':
      return 'Goroutines';
    case 'bytes':
      return 'Bytes';
    case 'lock_samples':
      return 'Lock Samples';
    case 'lock_nanoseconds':
      return 'Lock Nanoseconds';
    case 'trace_samples':
      return 'Trace Samples';
  }
};

export const isJSONFile = (file: File) =>
  file.name.match(/\.json$/) ||
  file.type === 'application/json' ||
  file.type === 'text/json';
