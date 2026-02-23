const grafanaConfig = require('@grafana/eslint-config/flat');
const importPlugin = require('eslint-plugin-import');
const cssModulesPlugin = require('eslint-plugin-css-modules');
const unusedImportsPlugin = require('eslint-plugin-unused-imports');
const globals = require('globals');

// globals package has a known bug where some keys have trailing whitespace
function trimGlobals(obj) {
  return Object.fromEntries(Object.entries(obj).map(([k, v]) => [k.trim(), v]));
}

module.exports = [
  // Ignores (replaces .eslintignore)
  {
    ignores: [
      'public/build/**',
      'scripts/**',
      'pkg/api/static/**',
      '**/dist',
      'jest.config.js',
      'cypress/**',
      'cypress.config.ts',
      'testSetupFile.js',
      'og/**',
      'examples/**',
      'public/app/util/**',
      '**.spec.ts*',
      'svg-transform.js',
      'setupAfterEnv.ts',
      'globalSetup.js',
      'globalTeardown.js',
      'jest-css-modules-transform-config.js',
      'cmd/profilecli/**',
      'tools/k6/**',
      'eslint.config.js',
    ],
  },

  // @grafana/eslint-config (includes react, react-hooks, prettier, typescript-eslint, jsdoc, stylistic)
  ...grafanaConfig,

  // eslint-plugin-import
  importPlugin.flatConfigs.recommended,
  importPlugin.flatConfigs.typescript,

  // Project config
  {
    files: ['**/*.js', '**/*.ts', '**/*.tsx'],
    plugins: {
      'css-modules': cssModulesPlugin,
      'unused-imports': unusedImportsPlugin,
    },
    languageOptions: {
      globals: {
        ...trimGlobals(globals.browser),
        ...trimGlobals(globals.jquery),
      },
      parserOptions: {
        project: ['tsconfig.json'],
      },
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
    rules: {
      'react/react-in-jsx-scope': 'error',
      'react-hooks/exhaustive-deps': 'error',
      'no-duplicate-imports': 'off',
      '@typescript-eslint/naming-convention': [
        'warn',
        { selector: 'function', format: ['PascalCase', 'camelCase'] },
      ],
      'import/no-duplicates': 'error',
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
          patterns: [],
        },
      ],
      'react/prop-types': ['off'],
      // New rules from react-hooks v7 — disable for now, enable as a follow-up
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/refs': 'off',
      'react-hooks/purity': 'off',
    },
  },
];
