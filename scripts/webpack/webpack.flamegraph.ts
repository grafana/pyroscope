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
    //    new BundleAnalyzerPlugin(),
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
    devtool: 'source-map',
    entry: {
      index: './src/index.node.ts',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../packages/pyroscope-flamegraph/dist'),
      libraryTarget: 'commonjs',
      filename: 'index.node.js',
    },
  },
  {
    ...common,
    target: 'web',
    mode: 'production',
    devtool: 'source-map',
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

    // These are the libraries we don't want to ship in the bundle
    // Instead we just assume they will be available
    // Then we tell our users to install them
    externals: {
      react: 'react',
      'true-myth': 'commonjs2 true-myth',
      '@szhsin/react-menu': 'commonjs2 @szhsin/react-menu',
    },
  },
];
