const Pyroscope = require('@pyroscope/nodejs');

module.exports = (config) => {
  process.env['PYROSCOPE_SAMPLING_DURATION'] = 1000;
  Pyroscope.init({ serverAddress: '_', appName: 'pyroscope-oss.frontend' });
  if (process.env.CI) {
    Pyroscope.startCpuProfiling();
  }
};
