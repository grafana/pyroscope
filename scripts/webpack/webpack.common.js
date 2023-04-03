const HtmlWebpackPlugin = require('html-webpack-plugin');
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
    alias: {
      '@webapp': path.resolve(
        __dirname,
        '../../node_modules/pyroscope-oss/webapp/javascript'
      ),
    },
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
    ],
  },
};
