import { determineDefaultApp } from './determineDefaultApp';

describe('async determineDefaultApp(apps)', () => {
  it('should return the first "cpu app"', async () => {
    const apps = [
      {
        name: 'pyroscope',
        __profile_type__: 'block:contentions:count::',
      },
      {
        name: 'nodejs-app',
        __profile_type__: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
      },
      {
        name: 'ride-sharing-app',
        __profile_type__: 'memory:alloc_space:bytes::',
      },
      {
        name: 'ride-sharing-app',
        __profile_type__: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
      },
    ];

    const result = await determineDefaultApp(apps);

    expect(result).toBe(apps[1]);
  });

  describe('if no "cpu app" is found', () => {
    it('should return the first ".itimer app"', async () => {
      const apps = [
        {
          name: 'pyroscope',
          __profile_type__: 'block:contentions:count::',
        },
        {
          name: 'nodejs-app',
          __profile_type__: 'foo:.itimer:bar',
        },
        {
          name: 'ride-sharing-app',
          __profile_type__: 'memory:alloc_space:bytes::',
        },
        {
          name: 'ride-sharing-app',
          __profile_type__: 'foo:.itimer:bar',
        },
      ];

      const result = await determineDefaultApp(apps);

      expect(result).toBe(apps[1]);
    });
  });

  describe('otherwise', () => {
    it('it should return the first app', async () => {
      const apps = [
        {
          name: 'pyroscope',
          __profile_type__: 'block:contentions:count::',
        },
        {
          name: 'ride-sharing-app',
          __profile_type__: 'memory:alloc_space:bytes::',
        },
      ];

      const result = await determineDefaultApp(apps);

      expect(result).toBe(apps[0]);
    });
  });
});
