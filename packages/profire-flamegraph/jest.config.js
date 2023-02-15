module.exports = {
  ...require('../../jest.config'),
  rootDir: '.',
  setupFilesAfterEnv: ['<rootDir>/setupAfterEnv.ts'],
};
