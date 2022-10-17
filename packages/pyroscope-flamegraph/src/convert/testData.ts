const specificFunctionName = 'specific-function-name';

export const tree = {
  name: 'name',
  key: '/name',
  self: [0],
  total: [1000],
  children: [
    {
      name: specificFunctionName,
      key: '/name/specific-function-name',
      self: [0],
      total: [600],
      children: [
        {
          name: specificFunctionName,
          key: '/name/specific-function-name/specific-function-name',
          self: [200],
          total: [200],
          children: [],
        },
        {
          name: 'name-3-2',
          key: '/name/specific-function-name/name-3-2',
          self: [400],
          total: [400],
          children: [],
        },
      ],
    },
    {
      name: 'name-2-2',
      key: '/name/name-2-2',
      self: [0],
      total: [400],
      children: [
        {
          name: 'name-3-1',
          key: '/name/name-2-2/name-3-1',
          self: [100],
          total: [400],
          children: [
            {
              name: specificFunctionName,
              key: '/name/name-2-2/name-3-1/specific-function-name',
              self: [0],
              total: [300],
              children: [
                {
                  name: 'name-5-1',
                  key: '/name/name-2-2/name-3-1/specific-function-name/name-5-1',
                  self: [150],
                  total: [150],
                  children: [],
                },
                {
                  name: 'name-5-2',
                  key: '/name/name-2-2/name-3-1/specific-function-name/name-5-2',
                  self: [150],
                  total: [150],
                  children: [],
                },
              ],
            },
          ],
        },
      ],
    },
  ],
};
