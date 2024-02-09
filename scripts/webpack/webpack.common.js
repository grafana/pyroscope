const CopyWebpackPlugin = require('copy-webpack-plugin');

const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');

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
      '@pyroscope': path.resolve(__dirname, '../../public/app'),
      // some sub-dependencies use a different version of @emotion/react and generate warnings
      // in the browser about @emotion/react loaded twice. We want to only load it once
      '@emotion/react': require.resolve('@emotion/react'),
      // Dependencies
      //...deps,
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
    new CopyWebpackPlugin({
      patterns: [
        {
          from: 'node_modules/@grafana/ui/dist/public/img/icons',
          to: 'grafana/img/icons/',
        },
      ],
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
        // exclude: /node_modules\/(?!pyroscope-oss).*/,
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
