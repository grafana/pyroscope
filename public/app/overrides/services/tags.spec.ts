import { queryToMatchers } from '@webapp/services/tags';

describe('queryToMatchers', () => {
  it('full query', () => {
    const query =
      'goroutine:goroutine:count:goroutine:count{service_name="cortex-dev-01/ruler", foo="bar"}';
    const matchers = queryToMatchers(query);

    expect(matchers).toEqual([
      '{__profile_type__="goroutine:goroutine:count:goroutine:count", service_name="cortex-dev-01/ruler", foo="bar"}',
    ]);
  });
  it('just profile type', () => {
    const query = 'goroutine:goroutine:count:goroutine:count';
    const matchers = queryToMatchers(query);

    expect(matchers).toEqual([
      '{__profile_type__="goroutine:goroutine:count:goroutine:count"}',
    ]);
  });

  it('just tags', () => {
    const query = '{service_name="cortex-dev-01/ruler", foo="bar"}';
    const matchers = queryToMatchers(query);

    expect(matchers).toEqual([
      '{service_name="cortex-dev-01/ruler", foo="bar"}',
    ]);
  });
});
