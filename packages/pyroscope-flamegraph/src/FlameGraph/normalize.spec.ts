import { normalize } from './normalize';
import { Flamebearer } from '@pyroscope/models/src';

describe('normalize', () => {
  it('accepts a flamebearer', () => {
    const flame: Flamebearer = {
      format: 'single',
      names: ['foo'],
      units: 'unknown',
      levels: [[99]],
      spyName: 'unknown',
      numTicks: 10,
      sampleRate: 100,
      maxSelf: 1,
    };

    expect(normalize({ flamebearer: flame })).toStrictEqual(flame);
  });

  it('accepts a profile', () => {
    const flame = normalize({
      profile: {
        metadata: {
          spyName: 'unknown',
          format: 'single',
          sampleRate: 100,
          units: 'unknown',
        },
        flamebearer: {
          levels: [[99]],
          maxSelf: 1,
          names: ['foo'],
          numTicks: 10,
        },
      },
    });

    expect(flame).toStrictEqual({
      format: 'single',
      names: ['foo'],
      units: 'unknown',
      levels: [[99]],
      spyName: 'unknown',
      numTicks: 10,
      sampleRate: 100,
      maxSelf: 1,
      rightTicks: undefined,
      leftTicks: undefined,
    });
  });

  it('accepts nothing', () => {
    const flame = normalize({});
    expect(flame).toStrictEqual({
      format: 'single',
      names: [],
      units: 'unknown',
      levels: [[]],
      spyName: 'unknown',
      numTicks: 0,
      sampleRate: 0,
      maxSelf: 0,
    });
  });
});
