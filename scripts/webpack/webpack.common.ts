import { getAlias, getJsLoader, getStyleLoaders } from './shared';

const webpack = require('webpack');
const path = require('path');
const glob = require('glob');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const CopyPlugin = require('copy-webpack-plugin');
// uncomment if you want to see the webpack bundle analysis
// const { BundleAnalyzerPlugin } = require('webpack-bundle-analyzer');
//

const fs = require('fs');

const pages = glob
  .sync('./webapp/templates/*.html')
  .map((x) => path.basename(x));
const pagePlugins = pages.map(
  (name) =>
    new HtmlWebpackPlugin({
      filename: path.resolve(__dirname, `../../webapp/public/${name}`),
      template: path.resolve(__dirname, `../../webapp/templates/${name}`),
      inject: false,
      chunksSortMode: 'none',
      templateParameters: (compilation, assets, options) => ({
        extra_metadata: process.env.EXTRA_METADATA
          ? fs.readFileSync(process.env.EXTRA_METADATA)
          : '',
        mode: process.env.NODE_ENV,
        webpack: compilation.getStats().toJson(),
        compilation,
        webpackConfig: compilation.options,
        htmlWebpackPlugin: {
          files: assets,
          options,
        },
      }),
    })
);

module.exports = {
  target: 'web',

  entry: {
    app: './webapp/javascript/index.jsx',
    styles: './webapp/sass/profile.scss',
  },

  output: {
    publicPath: '',
    path: path.resolve(__dirname, '../../webapp/public/assets'),
    filename: '[name].[hash].js',
    clean: true,
  },

  resolve: {
    extensions: ['.ts', '.tsx', '.es6', '.js', '.jsx', '.json', '.svg'],
    alias: getAlias(),
    modules: [
      'node_modules',
      path.resolve('webapp'),
      path.resolve('node_modules'),
    ],
  },

  stats: {
    children: false,
    warningsFilter: /export .* was not found in/,
    source: false,
  },

  watchOptions: {
    ignored: /node_modules/,
  },

  module: {
    // Note: order is bottom-to-top and/or right-to-left
    rules: [
      ...getJsLoader(),
      ...getStyleLoaders(),
      {
        test: /\.(svg|ico|jpg|jpeg|png|gif|eot|otf|webp|ttf|woff|woff2|cur|ani|pdf)(\?.*)?$/,
        loader: 'file-loader',

        // We output files to assets/static/img, where /assets comes from webpack's output dir
        // However, we still need to prefix the public URL with /assets/static/img
        options: {
          outputPath: 'static/img',
          publicPath: '/assets/static/img',
          name: '[name].[hash:8].[ext]',
        },
      },
    ],
  },

  plugins: [
    // uncomment if you want to see the webpack bundle analysis
    // new BundleAnalyzerPlugin(),
    new webpack.ProvidePlugin({
      $: 'jquery',
      jQuery: 'jquery',
    }),
    ...pagePlugins,
    new MiniCssExtractPlugin({
      filename: '[name].[hash].css',
    }),
    new webpack.IgnorePlugin({
      resourceRegExp: /^\.\/locale$/,
      contextRegExp: /moment$/,
    }),
    new CopyPlugin({
      patterns: [
        {
          from: 'webapp/images',
          to: 'images',
        },
      ],
    }),
  ],
};
