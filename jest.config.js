const path = require('path');
const { pathsToModuleNameMapper } = require('ts-jest');
const { compilerOptions } = require('./tsconfig');

module.exports = {
  // TypeScript files (.ts, .tsx) will be transformed by ts-jest to CommonJS syntax, and JavaScript files (.js, jsx) will be transformed by babel-jest.
  testEnvironment: 'jsdom',
  testMatch: ['**/?(*.)+(spec|test).+(ts|tsx|js)'],
  transform: {
    '\\.module\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.svg$': path.join(__dirname, 'svg-transform.js'),
    '^.+\\.(t|j)sx?$': ['@swc/jest'],
  },

  transformIgnorePatterns: [
    // force us to transpile these dependencies
    // https://stackoverflow.com/a/69150188
    'node_modules/(?!(true-myth|d3|d3-array|internmap|d3-scale|react-notifications-component|@react-hook))',
  ],

  testPathIgnorePatterns: ['/node_modules/', '/og/'],

  // Reuse the same modules from typescript
  moduleNameMapper: pathsToModuleNameMapper(compilerOptions.paths, {
    prefix: '<rootDir>',
    '@pyroscope/(.*)$': path.join(__dirname, 'public/app/$1'),
  }),

  globalSetup: '<rootDir>/globalSetup.js',
  globalTeardown: '<rootDir>/globalTeardown.js',
  setupFilesAfterEnv: [path.join(__dirname, 'setupAfterEnv.ts')],
};
