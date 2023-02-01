const Pyroscope = require('@pyroscope/nodejs');

module.exports = (config) => {
  Pyroscope.init({ serverAddress: '_', appName: 'pyroscope-oss.frontend' });
  if (process.env.CI) {
    Pyroscope.start();
  }
};
