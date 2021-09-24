const { merge } = require('webpack-merge');
const { BundleAnalyzerPlugin } = require('webpack-bundle-analyzer');

const common = require('./webpack.common');

module.exports = merge(common, {
  mode: 'development',
  plugins: [new BundleAnalyzerPlugin()],
});
