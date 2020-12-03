'use strict';

const merge = require('webpack-merge').merge;
const common = require('./webpack.common.js');
const webpack = require('webpack');
const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');

module.exports = (env = {}) =>
  merge(common, {
    mode: 'development',

    entry: {
      app: './webapp/javascript/index.tsx',
    },

    watchOptions: {
      ignored: /node_modules/,
    },

    module: {
      // Note: order is bottom-to-top and/or right-to-left
      rules: [
        {
          test: /\.tsx?$/,
          exclude: /node_modules/,
          use: [
            {
              loader: 'babel-loader',
              options: {
                cacheDirectory: true,
                babelrc: true,
                // Note: order is bottom-to-top and/or right-to-left
                presets: [
                  [
                    '@babel/preset-env',
                    {
                      targets: {
                        browsers: 'last 3 versions',
                      },
                      useBuiltIns: 'entry',
                      corejs: 3,
                      modules: false,
                    },
                  ],
                  [
                    '@babel/preset-typescript',
                    {
                      allowNamespaces: true,
                    },
                  ],
                  '@babel/preset-react',
                ],
              },
            },
          ],
        },
      ],
    },

    plugins: [
      new HtmlWebpackPlugin({
        filename: path.resolve(__dirname, '../../webapp/public/index.html'),
        template: path.resolve(__dirname, '../../webapp/templates/index.html'),
        inject: false,
        chunksSortMode: 'none',
        templateParameters: (compilation, assets, options) => {
          return ({
            webpack: compilation.getStats().toJson(),
            compilation: compilation,
            webpackConfig: compilation.options,
            htmlWebpackPlugin: {
              files: assets,
              options: options
            }
          })
        },
      }),
      new MiniCssExtractPlugin({
        filename: '[name].[hash].css',
      }),
      // new BundleAnalyzerPlugin({
      //   analyzerPort: 8889
      // })
    ],
  });
