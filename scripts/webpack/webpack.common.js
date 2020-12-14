const path = require('path');
const webpack = require('webpack');

const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const { CleanWebpackPlugin } = require('clean-webpack-plugin');

// const MonacoWebpackPlugin = require('monaco-editor-webpack-plugin');

let buildFlags = require('child_process')
  .execSync('scripts/generate-build-flags.sh ')
  .toString();

console.log(path.resolve());
module.exports = {
  target: 'web',
  entry: {
    app: './webapp/javascript/index.tsx',
    styles: './webapp/sass/profile.scss',
  },
  output: {
    path: path.resolve(__dirname, '../../webapp/public/build'),
    filename: '[name].[hash].js',
    // Keep publicPath relative for host.com/grafana/ deployments
    publicPath: 'webapp/public/build/',
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
      // we need full path to root node_modules for grafana-enterprise symlink to work
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
  plugins: [
    new webpack.ProvidePlugin({
      $: 'jquery',
      jQuery: 'jquery',
    }),
    new webpack.DefinePlugin({
      BUILD_FLAGS: JSON.stringify(buildFlags),
    }),
    new CleanWebpackPlugin(),
  ],
  module: {
    rules: [
      /**
       * Some npm packages are bundled with es2015 syntax, ie. debug
       * To make them work with PhantomJS we need to transpile them
       * to get rid of unsupported syntax.
       */
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
  // TODO: fix
  // https://webpack.js.org/plugins/split-chunks-plugin/#split-chunks-example-3
  // optimization: {
  //   moduleIds: 'hashed',
  //   runtimeChunk: 'single',
  //   splitChunks: {
  //     chunks: 'all',
  //     minChunks: 1,
  //     cacheGroups: {
  //       moment: {
  //         test: /[\\/]node_modules[\\/]moment[\\/].*[jt]sx?$/,
  //         chunks: 'initial',
  //         priority: 20,
  //         enforce: true,
  //       },
  //       angular: {
  //         test: /[\\/]node_modules[\\/]angular[\\/].*[jt]sx?$/,
  //         chunks: 'initial',
  //         priority: 50,
  //         enforce: true,
  //       },
  //       vendors: {
  //         test: /[\\/]node_modules[\\/].*[jt]sx?$/,
  //         chunks: 'initial',
  //         priority: -10,
  //         reuseExistingChunk: true,
  //         enforce: true,
  //       },
  //       default: {
  //         priority: -20,
  //         chunks: 'all',
  //         test: /.*[jt]sx?$/,
  //         reuseExistingChunk: true,
  //       },
  //     },
  //   },
  // },
};
