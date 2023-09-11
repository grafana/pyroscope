const path = require('path');

module.exports = {
  // TypeScript files (.ts, .tsx) will be transformed by ts-jest to CommonJS syntax, and JavaScript files (.js, jsx) will be transformed by babel-jest.
  testEnvironment: 'jsdom',
  setupFilesAfterEnv: [path.join(__dirname, 'setupAfterEnv.ts')],
  testMatch: [
    '**/__tests__/**/*.+(ts|tsx|js)',
    '**/?(*.)+(spec|test).+(ts|tsx|js)',
  ],
  moduleNameMapper: {
    '@webapp(.*)$': path.join(__dirname, 'webapp/javascript/$1'),
  },
  transform: {
    '\\.module\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.svg$': path.join(__dirname, 'svg-transform.js'),
    '^.+\\.(t|j)sx?$': ['@swc/jest'],
  },
  transformIgnorePatterns: [
    // force us to not transpile these dependencies
    // https://stackoverflow.com/a/69150188
    'node_modules/(?!(true-myth|d3|d3-array|internmap|d3-scale|react-notifications-component|graphviz-react|@react-hook))',
  ],
  globalSetup: '<rootDir>/globalSetup.js',
  globalTeardown: '<rootDir>/globalTeardown.js',
};
