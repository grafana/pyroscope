const path = require('path');

module.exports = {
  // TypeScript files (.ts, .tsx) will be transformed by ts-jest to CommonJS syntax, and JavaScript files (.js, jsx) will be transformed by babel-jest.
  testEnvironment: 'jsdom',
  //  setupFilesAfterEnv: [path.join(__dirname, 'setupAfterEnv.ts')],
  testMatch: ['**/?(*.)+(spec|test).+(ts|tsx|js)'],
  transform: {
    '^.+\\.(t|j)sx?$': ['@swc/jest'],
  },

  transformIgnorePatterns: [
    // force us to transpile these dependencies
    // https://stackoverflow.com/a/69150188
    'node_modules/(?!(true-myth|d3|d3-array|internmap|d3-scale|react-notifications-component|graphviz-react))',
  ],
};
