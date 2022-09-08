import { DataFrameView } from '@grafana/data';

export type Item = { level: number; value: number; label: string };
export type ItemWithStart = Item & { start: number };

export function nestedSetToLevels(dataView: DataFrameView<Item>): ItemWithStart[][] {
  const levels: ItemWithStart[][] = [];
  let offset = 0;

  for (let i = 0; i < dataView.length; i++) {
    const item = { ...dataView.get(i) };
    const prevItem = i > 0 ? { ...dataView.get(i - 1) } : undefined;

    levels[item.level] = levels[item.level] || [];
    if (prevItem && prevItem.level >= item.level) {
      const lastItem = levels[item.level][levels[item.level].length - 1];
      offset = lastItem.start + lastItem.value;
    }
    const newItem: ItemWithStart = {
      ...item,
      start: offset,
    };

    levels[item.level].push(newItem);
  }
  return levels;
}
