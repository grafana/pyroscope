import Color from 'color';

export interface TableValuesData {
  color?: Color;
  mean: number;
  stdDeviation: number;
  total: number;
  tagName: string;
  totalLabel: string;
  stdDeviationLabel: string;
  meanLabel: string;
}

export function addSpaces(maxLen: number, len: number, label: string) {
  if (!label.includes('.') || !maxLen || !len || len > maxLen) {
    return label;
  }

  return (
    '\xa0'.repeat(maxLen - len).concat(label.split('.')[0]) +
    '.'.concat(label.split('.')[1])
  );
}

export function getIntegerSpaceLengthForString(value?: string) {
  if (!value || !value.includes('.')) {
    return 1;
  }

  return value.split('.')[0].length;
}

export function getTableIntegerSpaceLengthByColumn(data: TableValuesData[]) {
  return data.reduce(
    (acc, current) => {
      const meanIntegerSpaceLength = getIntegerSpaceLengthForString(
        current.meanLabel
      );
      const stdDeviationIntegerSpaceLength = getIntegerSpaceLengthForString(
        current.stdDeviationLabel
      );
      const totalIntegerSpaceLength = getIntegerSpaceLengthForString(
        current.totalLabel
      );

      return {
        ...acc,
        mean:
          meanIntegerSpaceLength > acc.mean ? meanIntegerSpaceLength : acc.mean,
        stdDeviation:
          stdDeviationIntegerSpaceLength > acc.stdDeviation
            ? stdDeviationIntegerSpaceLength
            : acc.stdDeviation,
        total:
          totalIntegerSpaceLength > acc.total
            ? totalIntegerSpaceLength
            : acc.total,
      };
    },
    { mean: 1, stdDeviation: 1, total: 1 }
  );
}
