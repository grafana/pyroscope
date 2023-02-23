module.exports = (config) => {
  process.env['PYROSCOPE_SAMPLING_DURATION'] = 1000;
  const Pyroscope = require('@pyroscope/nodejs');

  Pyroscope.init({ serverAddress: '_', appName: 'pyroscope-oss.frontend' });
  if (process.env.CI) {
    Pyroscope.start();
  }
};
