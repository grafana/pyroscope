const { merge } = require('webpack-merge');
const common = require('./webpack.common');

module.exports = merge(common, {
  mode: 'production',

  // Recommended choice for production builds with high quality SourceMaps.
  devtool: 'source-map',
});
