'use strict';

const merge = require('webpack-merge').merge;
const webpack = require('webpack');
const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const { CleanWebpackPlugin } = require('clean-webpack-plugin');

module.exports = (env = {}) => {
  return {
    target: 'web',
    mode: 'development',

    entry: {
      app: './webapp/javascript/index.tsx',
      styles: './webapp/sass/profile.scss',
    },

    output: {
      path: path.resolve(__dirname, '../../webapp/public/build'),
      filename: '[name].[hash].js',
    },

    resolve: {
      extensions: ['.ts', '.tsx', '.es6', '.js', '.json', '.svg'],
      alias: {
        // rc-trigger uses babel-runtime which has internal dependency to core-js@2
        // this alias maps that dependency to core-js@t3
        'core-js/library/fn': 'core-js/stable',
      },
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

    node: {
      fs: 'empty',
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
        {
          test: /\.js$/,
          use: [
            {
              loader: 'babel-loader',
              options: {
                presets: [['@babel/preset-env']],
              },
            },
          ],
        },
        {
          test: /\.html$/,
          exclude: /(index|error)\.html/,
          use: [
            {
              loader: 'html-loader',
              options: {
                attrs: [],
                minimize: true,
                removeComments: false,
                collapseWhitespace: false,
              },
            },
          ],
        },
        {
          test: /\.css$/,
          // include: MONACO_DIR, // https://github.com/react-monaco-editor/react-monaco-editor
          use: ['style-loader', 'css-loader'],
        },
        {
          test: /\.scss$/,
          use: [
            MiniCssExtractPlugin.loader,
            {
              loader: 'css-loader',
              options: {
                importLoaders: 2,
                url: true,
                sourceMap: true
              },
            },
            {
              loader: 'postcss-loader',
              options: {
                sourceMap: true,
                config: { path: __dirname },
              },
            },
            {
              loader: 'sass-loader',
              options: {
                sourceMap: true
              },
            },
          ],
        },
        {
          test: /\.(svg|ico|jpg|jpeg|png|gif|eot|otf|webp|ttf|woff|woff2|cur|ani|pdf)(\?.*)?$/,
          loader: 'file-loader',
          options: { name: 'static/img/[name].[hash:8].[ext]' },
        },
      ],
    },

    plugins: [
      new webpack.ProvidePlugin({
        $: 'jquery',
        jQuery: 'jquery',
      }),
      new webpack.DefinePlugin({
        BUILD_FLAGS: JSON.stringify(buildFlags),
      }),
      new CleanWebpackPlugin({
        // cleanStaleWebpackAssets: false
      }),
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
  };
}
