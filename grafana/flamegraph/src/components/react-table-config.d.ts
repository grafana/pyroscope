// Remove this file when flamegraph merged into Grafana core

import type {
  UseSortByColumnProps,
  UseSortByState,
} from 'react-table';

declare module 'react-table' {
  export interface TableState<D extends Record<string, unknown> = Record<string, unknown>> extends UseSortByState<D> {}
  export interface ColumnInstance<D extends Record<string, unknown> = Record<string, unknown>> extends UseSortByColumnProps<D> {}
}
