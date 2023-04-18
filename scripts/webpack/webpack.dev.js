const { merge } = require('webpack-merge');
const common = require('./webpack.common');

module.exports = merge(common, {
  devtool: 'eval-source-map',
  mode: 'development',
  devServer: {
    port: 4040,
    historyApiFallback: true,
    proxy: {
      '/pyroscope': 'http://localhost:4100',
      '/querier.v1.QuerierService': 'http://localhost:4100',
    },
  },
  optimization: {
    runtimeChunk: 'single',
  },
});
