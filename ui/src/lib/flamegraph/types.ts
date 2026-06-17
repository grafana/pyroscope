import { type LevelItem } from './FlameGraph/dataTransform';

export { type FlameGraphDataContainer } from './FlameGraph/dataTransform';

export { type ExtraContextMenuButton } from './FlameGraph/FlameGraphContextMenu';

export type ClickedItemData = {
  posX: number;
  posY: number;
  label: string;
  item: LevelItem;
};

// Original sources used TS enums; switched to const objects + union types so
// the file remains erasable-only (project tsconfig has erasableSyntaxOnly).
export const SampleUnit = {
  Bytes: 'bytes',
  Short: 'short',
  Nanoseconds: 'ns',
} as const;
export type SampleUnit = (typeof SampleUnit)[keyof typeof SampleUnit];

export const SelectedView = {
  TopTable: 'topTable',
  FlameGraph: 'flameGraph',
  Both: 'both',
} as const;
export type SelectedView = (typeof SelectedView)[keyof typeof SelectedView];

export interface TableData {
  self: number;
  total: number;
}

export const ColorScheme = {
  ValueBased: 'valueBased',
  PackageBased: 'packageBased',
} as const;
export type ColorScheme = (typeof ColorScheme)[keyof typeof ColorScheme];

export type TextAlign = 'left' | 'right';
