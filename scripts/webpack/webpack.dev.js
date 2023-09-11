const { merge } = require('webpack-merge');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const common = require('./webpack.common');
const webpack = require('webpack');
const path = require('path');

module.exports = merge(common, {
  devtool: 'eval-source-map',
  mode: 'development',
  devServer: {
    port: 4041,
    historyApiFallback: true,
    proxy: {
      '/pyroscope': 'http://localhost:4040',
      '/querier.v1.QuerierService': 'http://localhost:4040',
    },
  },
  optimization: {
    runtimeChunk: 'single',
  },
  plugins: [
    new webpack.DefinePlugin({
      'process.env.BASEPATH': JSON.stringify('/'),
    }),
    // Duplicated in webpack.prod.js
    new HtmlWebpackPlugin({
      filename: path.resolve(__dirname, '../../public/build/index.html'),
      template: path.resolve(__dirname, '../../public/templates/index.html'),
      chunksSortMode: 'none',
    }),
  ],
});
