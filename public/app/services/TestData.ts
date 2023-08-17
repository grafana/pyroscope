import { Profile } from '@pyroscope/legacy/models';

const SimpleGoApp: Profile = {
  flamebearer: {
    names: [
      'total',
      'runtime.main',
      'main.slowFunction',
      'main.work',
      'main.main',
      'main.fastFunction',
    ],
    levels: [
      [0, 988, 0, 0],
      [0, 988, 0, 1],
      [0, 214, 0, 5, 0, 3, 2, 4, 0, 771, 0, 2],
      [0, 214, 214, 3, 2, 1, 1, 5, 0, 771, 771, 3],
    ],
    numTicks: 988,
    maxSelf: 771,
  },
  metadata: {
    format: 'single' as const,
    sampleRate: 100,
    spyName: 'gospy',
    units: 'samples',
  },
  version: 1,
};

export default SimpleGoApp;
