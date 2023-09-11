import { normalize } from './normalize';
import { Flamebearer, Profile } from '@pyroscope/legacy/models';

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
    const input: { profile: Profile } = {
      profile: {
        metadata: {
          spyName: 'unknown',
          format: 'single',
          sampleRate: 100,
          units: 'unknown',
        },
        flamebearer: {
          levels: [
            [0, 609, 0, 0],
            [0, 606, 0, 13, 0, 3, 0, 1],
          ],
          maxSelf: 1,
          names: ['total', 'foo'],
          numTicks: 10,
        },
      },
    };
    const snapshot = JSON.parse(JSON.stringify(input));

    const flame = normalize(input);

    expect(flame).toStrictEqual({
      format: 'single',
      names: ['total', 'foo'],
      units: 'unknown',
      levels: [
        [0, 609, 0, 0],
        [0, 606, 0, 13, 606, 3, 0, 1],
      ],
      spyName: 'unknown',
      numTicks: 10,
      sampleRate: 100,
      maxSelf: 1,
    });

    // It should not modify the original object
    // Since it's stored in the redux store
    expect(input).toStrictEqual(snapshot);
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
