const Pyroscope = require('@pyroscope/nodejs');

module.exports = async (config) => {
  if (process.env.CI) {
    await Pyroscope.stop();
  }
};
