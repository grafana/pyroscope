import { filterNonCPU } from './filterNonCPU';

describe('filterNonCPU', function () {
  describe('an app with cpu suffix is provided', function () {
    it('should be allowed', function () {
      expect(filterNonCPU('myapp.cpu')).toBe(true);
    });
  });

  describe('when an unidentified suffix is available', function () {
    it('should be allowed', function () {
      expect(filterNonCPU('myapp.weirdsuffix')).toBe(true);
    });
  });

  describe('an app with any other supported suffix is provided', function () {
    it.each`
      name                                 | expected
      ${'myapp.alloc_objects'}             | ${false}
      ${'myapp.alloc_space'}               | ${false}
      ${'myapp.goroutines'}                | ${false}
      ${'myapp.inuse_objects'}             | ${false}
      ${'myapp.inuse_space'}               | ${false}
      ${'myapp.mutex_count'}               | ${false}
      ${'myapp.alloc_in_new_tlab_bytes '}  | ${false}
      ${'myapp.alloc_in_new_tlab_objects'} | ${false}
      ${'myapp.lock_count'}                | ${false}
      ${'myapp.lock_duration'}             | ${false}
    `('filterNonCPU($appName) -> $expected', ({ name, expected }) => {
      expect(filterNonCPU(name)).toBe(expected);
    });
  });
});
