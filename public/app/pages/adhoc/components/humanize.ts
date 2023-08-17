/* eslint-disable default-case, consistent-return */
import { UnitsType } from '@pyroscope/legacy/models';
import { SpyNameFirstClassType } from '@pyroscope/legacy/models/spyName';

export const humanizeSpyname = (n: SpyNameFirstClassType) => {
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

export const humanizeUnits = (u: UnitsType) => {
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
    case 'exceptions':
      return 'Exceptions';
  }
};

export const isJSONFile = (file: File) =>
  file.name.match(/\.json$/) ||
  file.type === 'application/json' ||
  file.type === 'text/json';
