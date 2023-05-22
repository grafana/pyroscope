const Pyroscope = require('@pyroscope/nodejs');

module.exports = (config) => {
  if (process.env.CI) {
    Pyroscope.stopCpuProfiling();
    Pyroscope.stopHeapProfiling();
  }
};
