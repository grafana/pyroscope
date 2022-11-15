import type { TreeNode } from './flamebearersToTree';

export const tree: TreeNode = {
  name: 'name',
  key: '/name',
  self: [0],
  total: [1000],
  children: [
    {
      name: 'specific-function-name',
      key: '/name/specific-function-name',
      self: [0],
      total: [600],
      children: [
        {
          name: 'specific-function-name',
          key: '/name/specific-function-name/specific-function-name',
          self: [200],
          total: [200],
          children: [],
        },
        {
          name: 'wwwwwww',
          key: '/name/specific-function-name/wwwwwww',
          self: [20],
          total: [400],
          children: [
            {
              name: 'name-3-2',
              key: '/name/specific-function-name/wwwwwww/name-3-2',
              self: [380],
              total: [380],
              children: [],
            },
          ],
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
              name: 'specific-function-name',
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

const singleAppearanceZeroCalleesTree = {
  name: 'name-5-2',
  children: [],
  self: [0],
  total: [150],
  key: '/name-5-2',
};
const singleAppearanceSingleCalleeTree = {
  name: 'wwwwwww',
  key: '/wwwwwww',
  self: [0],
  total: [400],
  children: [
    {
      name: 'name-3-2',
      key: '/wwwwwww/name-3-2',
      self: [380],
      total: [380],
      children: [],
    },
  ],
};
const singleAppearanceMultipleCalleesTree = {
  name: 'name-2-2',
  key: '/name-2-2',
  total: [400],
  self: [0],
  children: [
    {
      name: 'name-3-1',
      key: '/name-2-2/name-3-1',
      self: [100],
      total: [400],
      children: [
        {
          name: 'specific-function-name',
          key: '/name-2-2/name-3-1/specific-function-name',
          self: [0],
          total: [300],
          children: [
            {
              name: 'name-5-1',
              key: '/name-2-2/name-3-1/specific-function-name/name-5-1',
              self: [150],
              total: [150],
              children: [],
            },
            {
              name: 'name-5-2',
              key: '/name-2-2/name-3-1/specific-function-name/name-5-2',
              self: [150],
              total: [150],
              children: [],
            },
          ],
        },
      ],
    },
  ],
};

export const singleAppearanceTrees = {
  zero: singleAppearanceZeroCalleesTree,
  one: singleAppearanceSingleCalleeTree,
  multiple: singleAppearanceMultipleCalleesTree,
};
