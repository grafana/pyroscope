import { SpyName } from './spyName';
import { Units } from './units';
/**
 * @deprecated one should use the Profile model
 */
export type Flamebearer = {
  /**
   * List of names
   */
  names: string[];
  /**
   * List of level
   *
   * This is NOT the same as in the flamebearer
   * that we receive from the server.
   * As in there are some transformations required
   * (see deltaDiffWrapper)
   */
  levels: number[][];
  numTicks: number;
  maxSelf: number;

  /**
   * Sample Rate, used in text information
   */
  sampleRate: number;
  units: Units;

  spyName: SpyName;
  // format: 'double' | 'single';
  //  leftTicks?: number;
  //  rightTicks?: number;
} & addTicks;

export type addTicks =
  | { format: 'double'; leftTicks: number; rightTicks: number }
  | { format: 'single' };

export const singleFF = {
  format: 'single' as const,
  jStep: 4,
  jName: 3,
  getBarOffset: (level: number[], j: number) => level[j],
  getBarTotal: (level: number[], j: number) => level[j + 1],
  getBarTotalDiff: (_: number[], __: number) => 0,
  getBarSelf: (level: number[], j: number) => level[j + 2],
  getBarSelfDiff: (_: number[], __: number) => 0,
  getBarName: (level: number[], j: number) => level[j + 3],
};

export const doubleFF = {
  format: 'double' as const,
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

// createFF returns an accesser for flamebearer
// TODO: rename it?
export function createFF(
  viewType: 'single' | 'double'
): typeof singleFF | typeof doubleFF {
  switch (viewType) {
    case 'single': {
      return singleFF;
    }
    case 'double': {
      return doubleFF;
    }

    default: {
      throw new Error(`Unsupported type: '${viewType}'`);
    }
  }
}
