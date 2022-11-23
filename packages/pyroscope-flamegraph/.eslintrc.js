const path = require('path');

module.exports = {
  extends: [path.join(__dirname, '../../.eslintrc.js')],
  ignorePatterns: [
    'babel.config.js',
    'jest.config.js',
    'setupAfterEnv.ts',
    '*.spec.*',
    '.eslintrc.js',
    // This file is not actually bundled
    // TODO move it to ./testFixtures or something
    'src/FlameGraph/FlameGraphComponent/testData.ts',
  ],

  rules: {
    // https://github.com/import-js/eslint-plugin-import/issues/1650
    'import/no-extraneous-dependencies': [
      'error',
      {
        packageDir: [process.cwd(), path.resolve(__dirname, '../../')],
      },
    ],
  },
  parserOptions: {
    tsconfigRootDir: __dirname,
  },
};
