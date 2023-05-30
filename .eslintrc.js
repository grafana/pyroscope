module.exports = {
  extends: [
    '@grafana/eslint-config',
    'plugin:import/recommended',
    'plugin:import/typescript',
  ],
  plugins: ['unused-imports'],
  rules: {
    'react/react-in-jsx-scope': 'error',
    'react-hooks/exhaustive-deps': 'error',
    'no-duplicate-imports': 'off',
    '@typescript-eslint/no-duplicate-imports': 'error',
    '@typescript-eslint/no-unused-vars': 'off',
    'unused-imports/no-unused-imports': 'error',
    'unused-imports/no-unused-vars': [
      'error',
      {
        vars: 'all',
        varsIgnorePattern: '^_',
        args: 'after-used',
        argsIgnorePattern: '^_',
      },
    ],
    'import/no-relative-packages': 'error',
    'no-restricted-imports': [
      'error',
      {
        patterns: [
          {
            group: ['../*', './*'],
            message:
              'Usage of relative parent imports is not allowed. Please use absolute(use alias) imports instead.',
          },
        ],
      },
    ],
  },
  env: {
    browser: true,
    jquery: true,
  },
  settings: {
    'import/internal-regex': '^@webapp',
    'import/resolver': {
      node: {
        extensions: ['.ts', '.tsx', '.es6', '.js', '.json', '.svg'],
      },
      typescript: {
        project: 'tsconfig.json',
      },
    },
  },
  parserOptions: {
    project: ['tsconfig.json'],
  },
  overrides: [
    {
      // For tests it's fine to import with ./myfile, since tests won't be overriden downstream
      files: ['*.spec.tsx', '*.spec.ts'],
      rules: {
        'no-restricted-imports': 'off',
      },
    },
  ],
};
