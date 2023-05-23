const HtmlWebpackPlugin = require('html-webpack-plugin');
const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const {
  dependencies: pyroOSSDeps,
} = require('../../node_modules/pyroscope-oss/package.json');
const webpack = require('webpack');
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
    // When using TsconfigPathsPlugin, paths overrides don't work
    // For example, setting a) '@webapp/*' and  b) '@webapp/components/ExportData'
    // Would end up ignoring b)
    alias: {
      // Specific Component overrides
      '@webapp/components/ExportData': path.resolve(
        __dirname,
        '../../public/app/overrides/components/ExportData'
      ),
      '@webapp/components/TimelineChart/ContextMenu.plugin': path.resolve(
        __dirname,
        '../../public/app/overrides/components/TimelineChart/ContextMenu.plugin'
      ),
      '@webapp/components/AppSelector/Label': path.resolve(
        __dirname,
        '../../public/app/overrides/components/AppSelector/Label'
      ),
      '@webapp/ui/Sidebar': path.resolve(
        __dirname,
        '../../public/app/overrides/ui/Sidebar'
      ),
      '@webapp/util/baseurl': path.resolve(
        __dirname,
        '../../public/app/overrides/util/baseurl'
      ),

      '@webapp/redux/store': path.resolve(
        __dirname,
        '../../public/app/redux/store'
      ),
      '@webapp/redux/hooks': path.resolve(
        __dirname,
        '../../public/app/redux/hooks'
      ),
      '@webapp/services/apps': path.resolve(
        __dirname,
        '../../public/app/overrides/services/appNames'
      ),
      '@webapp/services/render': path.resolve(
        __dirname,
        '../../public/app/overrides/services/render'
      ),
      '@webapp/services/tags': path.resolve(
        __dirname,
        '../../public/app/overrides/services/tags'
      ),
      '@webapp/services/base': path.resolve(
        __dirname,
        '../../public/app/overrides/services/base'
      ),

      // Common
      '@pyroscope/webapp': path.resolve(
        __dirname,
        '../../node_modules/pyroscope-oss/webapp'
      ),
      '@pyroscope/flamegraph': path.resolve(
        __dirname,
        '../../node_modules/pyroscope-oss/packages/pyroscope-flamegraph'
      ),
      '@pyroscope/models': path.resolve(
        __dirname,
        '../../node_modules/pyroscope-oss/packages/pyroscope-models'
      ),

      '@webapp': path.resolve(
        __dirname,
        '../../node_modules/pyroscope-oss/webapp/javascript'
      ),

      '@phlare': path.resolve(__dirname, '../../public/app'),
      // Dependencies
      ...deps,
    },
    plugins: [
      // Use same alias from tsconfig.json
      //      new TsconfigPathsPlugin({
      //        logLevel: 'info',
      //        // TODO: figure out why it could not use the baseUrl from tsconfig.json
      //        baseUrl: path.resolve(__dirname, '../../'),
      //      }),
    ],
  },
  ignoreWarnings: [/export .* was not found in/],
  stats: {
    children: false,
    source: false,
  },
  plugins: [
    new MiniCssExtractPlugin({
      filename: '[name].[contenthash].css',
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
