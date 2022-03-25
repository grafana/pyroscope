const path = require('path');

module.exports = {
  // TypeScript files (.ts, .tsx) will be transformed by ts-jest to CommonJS syntax, and JavaScript files (.js, jsx) will be transformed by babel-jest.
  preset: 'ts-jest/presets/js-with-babel',
  testEnvironment: 'jsdom',
  setupFilesAfterEnv: [path.join(__dirname, 'setupAfterEnv.ts')],
  testMatch: [
    '**/__tests__/**/*.+(ts|tsx|js)',
    '**/?(*.)+(spec|test).+(ts|tsx|js)',
  ],
  moduleNameMapper: {
    '@webapp(.*)$': path.join(__dirname, 'webapp/javascript/$1'),
    //    '@utils(.*)$': path.join(__dirname, 'webapp/javascript/util/$1'),
    //    '@models(.*)$': path.join(__dirname, 'webapp/javascript/models/$1'),
    //    '@ui(.*)$': path.join(__dirname, 'webapp/javascript/ui/$1'),
    //    '@pyroscope/redux(.*)$': path.join(__dirname, 'webapp/javascript/redux/$1'),
    //    '@pyroscope/services(.*)$': path.join(
    //      __dirname,
    //      'webapp/javascript/services/$1'
    //    ),
  },
  transform: {
    '\\.module\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.svg$': path.join(__dirname, 'svg-transform.js'),
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
