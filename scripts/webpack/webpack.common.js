const HtmlWebpackPlugin = require('html-webpack-plugin');
const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const {
  dependencies: pyroOSSDeps,
} = require('../../node_modules/pyroscope-oss/package.json');
const TsconfigPathsPlugin = require('tsconfig-paths-webpack-plugin');

// this is so that we don't import dependencies twice, once from pyroscope-oss and another from here
const deps = Object.entries(pyroOSSDeps).reduce((prev, [name]) => {
  return {
    ...prev,
    [name]: path.resolve(__dirname, `../../node_modules/${name}`),
  };
}, {});

module.exports = {
  target: 'web',
  entry: {
    app: './public/app/app.tsx',
  },
  output: {
    clean: true,
    path: path.resolve(__dirname, '../../public/build'),
    filename: '[name].[contenthash].js',
    publicPath: '',
  },
  resolve: {
    extensions: ['.ts', '.tsx', '.es6', '.js', '.json', '.svg'],
    modules: ['node_modules', path.resolve('public')],

    // TODO: unify with tsconfig.json
    alias: {
      // Dependencies
      ...deps,
    },
    plugins: [
      // Use same alias from tsconfig.json
      new TsconfigPathsPlugin({
        logLevel: 'info',
        // TODO: figure out why it could not use the baseUrl from tsconfig.json
        baseUrl: path.resolve(__dirname, '../../'),
      }),
    ],
  },
  ignoreWarnings: [/export .* was not found in/],
  stats: {
    children: false,
    source: false,
  },
  plugins: [
    new MiniCssExtractPlugin({}),
    new HtmlWebpackPlugin({
      filename: path.resolve(__dirname, '../../public/build/index.html'),
      template: path.resolve(__dirname, '../../public/templates/index.html'),
      chunksSortMode: 'none',
    }),
  ],
  module: {
    rules: [
      // CSS
      {
        test: /\.(css|scss)$/,
        use: [
          MiniCssExtractPlugin.loader,
          {
            loader: 'css-loader',
            options: {
              importLoaders: 2,
              url: true,
            },
          },
          {
            loader: 'sass-loader',
            options: {},
          },
        ],
      },

      {
        test: require.resolve('jquery'),
        loader: 'expose-loader',
        options: {
          exposes: ['$', 'jQuery'],
        },
      },
      {
        test: /\.(js|ts)x?$/,
        // Ignore everything except pyroscope-oss, since it's used as if it was local code
        exclude: /node_modules\/(?!pyroscope-oss).*/,
        use: [
          {
            loader: 'esbuild-loader',
            options: {
              loader: 'tsx',
              target: 'es2015',
            },
          },
        ],
      },

      // SVG
      {
        test: /\.svg$/,
        use: [
          {
            loader: 'react-svg-loader',
            options: {
              svgo: {
                plugins: [
                  { convertPathData: { noSpaceAfterFlags: false } },
                  { removeViewBox: false },
                ],
              },
            },
          },
        ],
      },
    ],
  },
};
