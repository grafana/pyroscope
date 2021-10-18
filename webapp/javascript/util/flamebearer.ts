/* eslint-disable import/prefer-default-export */
function createFF(viewType: 'single' | 'double') {
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

export { createFF };
