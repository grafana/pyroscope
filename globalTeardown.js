const Pyroscope = require('@pyroscope/nodejs');

module.exports = (config) => {
  if (process.env.CI) {
    console.time('stopping');
    Pyroscope.stopCpuProfiling();
    Pyroscope.stopHeapProfiling();
    console.timeEnd('stopping');
  }
};
