// not entirely sure where this should fit

export function getRatios(
  // Just to provide some help, so that people don't run getRatios on viewType 'single'
  viewType: 'double',
  level: number[],
  j: number,
  leftTicks: number,
  rightTicks: number
) {
  const ff = formatDouble;

  // throw an error
  // since otherwise there's no way to calculate a diff
  if (!leftTicks || !rightTicks) {
    // ideally this should never happen
    // however there must be a race condition caught in CI
    // https://github.com/pyroscope-io/pyroscope/pull/439/checks?check_run_id=3808581168
    console.error(
      "Properties 'rightTicks' and 'leftTicks' are required. Can't calculate ratio."
    );
    return { leftRatio: 0, rightRatio: 0 };
  }

  const leftRatio = ff.getBarTotalLeft(level, j) / leftTicks;
  const rightRatio = ff.getBarTotalRght(level, j) / rightTicks;

  return { leftRatio, rightRatio };
}

export function createFF(viewType: 'single' | 'diff' | 'double') {
  switch (viewType) {
    case 'single': {
      return formatSingle;
    }

    case 'double': {
      return formatDouble;
    }

    default:
      throw new Error(`Format not supported: '${viewType}'`);
  }
}

const formatSingle = {
  format: 'single',
  jStep: 4,
  jName: 3,
  getBarOffset: (level: number[], j: number) => level[j],
  getBarTotal: (level: number[], j: number) => level[j + 1],
  getBarTotalDiff: (level: number[], j: number) => 0,
  getBarSelf: (level: number[], j: number) => level[j + 2],
  getBarSelfDiff: (level: number[], j: number) => 0,
  getBarName: (level: number[], j: number) => level[j + 3],
};

const formatDouble = {
  format: 'double',
  jStep: 7,
  jName: 6,
  getBarOffset: (level: number[], j: number) => level[j] + level[j + 3],
  getBarTotal: (level: number[], j: number) => level[j + 4] + level[j + 1],
  getBarTotalLeft: (level: number[], j: number) => level[j + 1],
  getBarTotalRght: (level: number[], j: number) => level[j + 4],
  getBarTotalDiff: (level: number[], j: number) => {
    return level[j + 4] - level[j + 1];
  },
  getBarSelf: (level: number[], j: number) => level[j + 5] + level[j + 2],
  getBarSelfLeft: (level: number[], j: number) => level[j + 2],
  getBarSelfRght: (level: number[], j: number) => level[j + 5],
  getBarSelfDiff: (level: number[], j: number) => level[j + 5] - level[j + 2],
  getBarName: (level: number[], j: number) => level[j + 6],
};
