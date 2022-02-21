const path = require('path');

module.exports = {
  // TypeScript files (.ts, .tsx) will be transformed by ts-jest to CommonJS syntax, and JavaScript files (.js, jsx) will be transformed by babel-jest.
  preset: 'ts-jest/presets/js-with-babel',
  testEnvironment: 'jsdom',
  setupFilesAfterEnv: ['<rootDir>/setupAfterEnv.ts'],
  testMatch: [
    '**/__tests__/**/*.+(ts|tsx|js)',
    '**/?(*.)+(spec|test).+(ts|tsx|js)',
  ],
  moduleNameMapper: {
    '@utils(.*)$': '<rootDir>/webapp/javascript/util/$1',
    '@models(.*)$': '<rootDir>/webapp/javascript/models/$1',
    '@ui(.*)$': '<rootDir>/webapp/javascript/ui/$1',
    '@pyroscope/redux(.*)$': '<rootDir>/webapp/javascript/redux/$1',
    '@pyroscope/services(.*)$': '<rootDir>/webapp/javascript/services/$1',
  },
  transform: {
    '\\.module\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.svg$': '<rootDir>/svg-transform.js',
  },
  transformIgnorePatterns: [
    // force us to not transpile these dependencies
    // https://stackoverflow.com/a/69150188
    'node_modules/(?!(true-myth|d3|d3-array|internmap|d3-scale|react-notifications-component))',
  ],
  globals: {
    'ts-jest': {
      tsconfig: path.join(__dirname, `tsconfig.test.json`),
      diagnostics: {
        // https://github.com/kulshekhar/ts-jest/issues/1647#issuecomment-832577036
        pathRegex: /\.(test)\.tsx$/,
      },
    },
  },
};
