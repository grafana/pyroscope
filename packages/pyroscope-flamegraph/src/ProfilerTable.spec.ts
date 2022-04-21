// @ts-ignore
import { generateTable } from './ProfilerTable';

const render2 = {
  flamebearer: {
    names: [
      'cpu_function_0',
      'cpu_function_1',
      'cpu_function_2',
      'cpu_function_3',
    ],
    levels: [
      [32, 508, 100, 0],
      [32, 508, 100, 1],
      [32, 508, 100, 2],
      [32, 508, 100, 2],
      [32, 508, 100, 3],
    ],
    numTicks: 50,
    maxSelf: 58,
    spyName: 'gospy',
    sampleRate: 100,
    units: 'samples',
  },
  metadata: {
    sampleRate: 100,
    spyName: 'gospy',
    units: 'samples',
  },
  timeline: {
    startTime: 1631138160,
    samples: [1, 3, 1, 2, 2, 4, 2, 3, 1, 1, 3, 4],
    durationDelta: 10,
  },
};

describe('table calculations', () => {
  it('should generate correct table on recursive calls', () => {
    const table = generateTable({
      ...render2.flamebearer,
      format: 'single',
      leftTicks: 5,
      rightTicks: 5,
    });
    expect(table).toEqual([
      { name: 'cpu_function_0', self: 100, total: 508 },
      { name: 'cpu_function_1', self: 100, total: 508 },
      { name: 'cpu_function_2', self: 200, total: 508 }, // this line shouldn't be counted twice
      { name: 'cpu_function_3', self: 100, total: 508 },
    ]);
  });
});
