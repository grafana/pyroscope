import path from 'path';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import { ESBuildMinifyPlugin } from 'esbuild-loader';
import webpack from 'webpack';
import { getAlias, getJsLoader, getStyleLoaders } from './shared';

const common = {
  mode: 'production',
  devtool: 'source-map',

  resolve: {
    extensions: ['.ts', '.tsx', '.es6', '.js', '.jsx', '.json', '.svg'],
    alias: getAlias(),
    modules: [
      'node_modules',
      path.resolve('webapp'),
      path.resolve('node_modules'),
    ],
  },

  watchOptions: {
    ignored: /node_modules/,
  },

  optimization: {
    minimizer: [
      new ESBuildMinifyPlugin({
        target: 'es2015',
        css: true,
      }),
    ],
  },

  module: {
    // Note: order is bottom-to-top and/or right-to-left
    rules: [
      ...getJsLoader(),
      ...getStyleLoaders(),
      {
        test: /\.svg$/,
        use: [
          { loader: 'babel-loader' },
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

  plugins: [
    new MiniCssExtractPlugin({}),
    new webpack.DefinePlugin({
      'process.env': {
        PYROSCOPE_HIDE_LOGO: process.env['PYROSCOPE_HIDE_LOGO'],
      },
    }),
  ],
};

export default [
  {
    ...common,
    target: 'node',
    mode: 'production',
    // devtool: 'source-map',
    entry: {
      index: './src/index.node.ts',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../packages/pyroscope-flamegraph/dist'),
      libraryTarget: 'commonjs',
      filename: 'index.node.js',
    },

    externals: {
      react: 'react',
      'react-dom': 'react-dom',
    },
  },
  {
    ...common,
    target: 'web',
    mode: 'production',
    // devtool: 'source-map',
    entry: {
      index: './src/index.tsx',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../packages/pyroscope-flamegraph/dist'),
      libraryTarget: 'umd',
      library: 'pyroscope',
      filename: 'index.js',
      globalObject: 'this',
    },

    externals: {
      react: 'react',
      'react-dom': 'react-dom',
    },
  },
];
