import { formatAsOBject } from '@pyroscope/util/formatDate';
import { Selection } from '../markings';

export const getSelectionBoundaries = (selection: Selection) => {
  if (selection.from.startsWith('now') || selection.to.startsWith('now')) {
    return {
      from: new Date(formatAsOBject(selection.from)).getTime(),
      to: new Date(formatAsOBject(selection.to)).getTime(),
    };
  }

  return {
    from:
      selection.from.length === 10
        ? Number(selection.from) * 1000
        : Number(selection.from),
    to:
      selection.to.length === 10
        ? Number(selection.to) * 1000
        : Number(selection.to),
  };
};
