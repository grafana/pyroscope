const { merge } = require('webpack-merge');
const webpack = require('webpack');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const path = require('path');
const common = require('./webpack.common');

module.exports = merge(common, {
  mode: 'production',
  output: {
    clean: true,
    path: path.resolve(__dirname, '../../public/build/assets'),
    publicPath: 'assets',
  },
  plugins: [
    new webpack.DefinePlugin({
      // The go server will parse this HTML file
      'process.env.BASEPATH': JSON.stringify('{{ .BaseURL }}'),
    }),
    // Duplicated in webpack.dev.js
    new HtmlWebpackPlugin({
      filename: path.resolve(__dirname, '../../public/build/index.html'),
      template: path.resolve(__dirname, '../../public/templates/index.html'),
      chunksSortMode: 'none',
    }),
  ],
});
