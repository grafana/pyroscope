module.exports = {
  extends: [
    '@grafana/eslint-config',
    'plugin:import/recommended',
    'plugin:import/typescript',
  ],
  plugins: ['unused-imports'],
  rules: {
    'react-hooks/exhaustive-deps': 'warn',
    'no-duplicate-imports': 'off',
    '@typescript-eslint/no-duplicate-imports': 'error',
    '@typescript-eslint/no-unused-vars': 'off',
    'unused-imports/no-unused-imports': 'error',
    'unused-imports/no-unused-vars': [
      'warn',
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
};
