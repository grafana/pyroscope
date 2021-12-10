/** @type {import('ts-jest/dist/types').InitialOptionsTsJest} */
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
  },
  transform: {
    '\\.module\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.(css|scss)$': 'jest-css-modules-transform',
    '\\.svg$': 'svg-jest',
  },
  globals: {
    'ts-jest': {
      diagnostics: {
        // https://github.com/kulshekhar/ts-jest/issues/1647#issuecomment-832577036
        pathRegex: /\.(test)\.tsx$/,
      },
    },
  },
};
