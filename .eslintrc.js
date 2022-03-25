const path = require('path');

module.exports = {
  plugins: ['prettier', 'css-modules', 'import'],
  extends: [
    'airbnb-typescript-prettier',
    'plugin:cypress/recommended',
    'plugin:import/typescript',
    'prettier',
    'prettier/react',
    'plugin:css-modules/recommended',
  ],
  rules: {
    '@typescript-eslint/no-unused-vars': 'warn',
    '@typescript-eslint/no-shadow': 'warn',

    // https://stackoverflow.com/questions/63818415/react-was-used-before-it-was-defined/64024916#64024916
    'no-use-before-define': ['off'],
    '@typescript-eslint/no-use-before-define': 'warn',

    // react functional components are usually written using PascalCase
    '@typescript-eslint/naming-convention': [
      'warn',
      { selector: 'function', format: ['PascalCase', 'camelCase'] },
    ],
    '@typescript-eslint/no-empty-function': 'warn',
    '@typescript-eslint/no-var-requires': 'warn',
    'react-hooks/exhaustive-deps': 'warn',

    'import/no-extraneous-dependencies': ['error', { devDependencies: true }],
    'no-param-reassign': ['warn'],
    'no-case-declarations': ['warn'],
    'no-restricted-globals': ['warn'],
    'react/button-has-type': ['warn'],
    'react/prop-types': ['off'],
    'jsx-a11y/heading-has-content': ['warn'],
    'jsx-a11y/control-has-associated-label': ['warn'],
    'no-undef': ['warn'],
    'jsx-a11y/mouse-events-have-key-events': ['warn'],
    'jsx-a11y/click-events-have-key-events': ['warn'],
    'jsx-a11y/no-static-element-interactions': ['warn'],
    'jsx-a11y/label-has-associated-control': [
      'error',
      {
        required: {
          some: ['nesting', 'id'],
        },
      },
    ],
    'react/jsx-filename-extension': [1, { extensions: ['.tsx', '.ts'] }],
    'import/extensions': [
      'error',
      'always',
      {
        js: 'never',
        jsx: 'never',
        ts: 'never',
        tsx: 'never',
      },
    ],
    'spaced-comment': [2, 'always', { exceptions: ['*'] }],
    'react/require-default-props': 'off',

    'import/no-extraneous-dependencies': [
      'error',
      {
        devDependencies: [
          '**/*.spec.jsx',
          '**/*.spec.ts',
          '**/*.spec.tsx',
          '**/*.stories.tsx',
        ],
        packageDir: [
          // TODO compute this dynamically
          path.resolve(__dirname, 'packages/pyroscope-flamegraph'),
          process.cwd(),
        ],
      },
    ],
    // otherwise it conflincts with ts411
    'dot-notation': 'off',

    // disable relative imports to force people to use '@webapp'
    'import/no-relative-packages': 'error',
  },
  env: {
    browser: true,
    //    node: true,
    jquery: true,
  },
  parserOptions: {
    project: './tsconfig.eslint.json',
  },
  settings: {
    'import/internal-regex': '^@pyroscope',
    'import/resolver': {
      'eslint-import-resolver-lerna': {
        packages: path.resolve(__dirname, 'packages'),
      },
      webpack: {
        config: path.join(__dirname, 'scripts/webpack/webpack.common.ts'),
      },
    },
  },
  overrides: [
    // Tests are completely different
    // And we shouldn't be so strict
    {
      files: ['**/?(*.)+(spec|test).+(ts|tsx|js)'],
      plugins: ['jest'],
      env: {
        node: true,
        'jest/globals': true,
      },
    },
  ],
};
