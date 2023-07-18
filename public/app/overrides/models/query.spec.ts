import { brandQuery, parse } from '@webapp/models/query';

const cases: Array<
  [string, { profileId: string; tags?: Record<string, string> } | undefined]
> = [
  ['{}', undefined],
  [
    'process_cpu:cpu:nanoseconds:cpu:nanoseconds{}',
    { profileId: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds' },
  ],
  [
    'process_cpu:cpu:nanoseconds:cpu:nanoseconds{tag="mytag"}',
    {
      profileId: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
      tags: {
        tag: 'mytag',
      },
    },
  ],
  [
    'process_cpu:cpu:nanoseconds:cpu:nanoseconds{tag="mytag", tag2="mytag2"}',
    {
      profileId: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
      tags: {
        tag: 'mytag',
        tag2: 'mytag2',
      },
    },
  ],
  [
    'process_cpu:cpu:nanoseconds:cpu:nanoseconds{tag="mytag",tag2="mytag2"}',
    {
      profileId: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
      tags: {
        tag: 'mytag',
        tag2: 'mytag2',
      },
    },
  ],
  [
    'process_cpu:cpu:nanoseconds:cpu:nanoseconds{tag="my.ta/g_"}',
    {
      profileId: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
      tags: {
        tag: 'my.ta/g_',
      },
    },
  ],
];

test.each(cases)('parse(%s) should be %s', (query, expected) => {
  expect(parse(brandQuery(query))).toEqual(expected);
});
