/** @type {import('ts-jest/dist/types').InitialOptionsTsJest} */
module.exports = {
  // TypeScript files (.ts, .tsx) will be transformed by ts-jest to CommonJS syntax, and JavaScript files (.js, jsx) will be transformed by babel-jest.
  preset: 'ts-jest/presets/js-with-babel',
  testEnvironment: 'jsdom',
  setupFilesAfterEnv: ['<rootDir>/setupAfterEnv.js'],
  testMatch: [
    '**/__tests__/**/*.+(ts|tsx|js)',
    '**/?(*.)+(spec|test).+(ts|tsx|js)',
  ],

  transform: {
    '\\.module\\.css$': 'jest-css-modules-transform',
  },
};
