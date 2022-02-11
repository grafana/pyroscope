import path from 'path';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import { ESBuildMinifyPlugin } from 'esbuild-loader';
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
      { test: /\.svg$/, loader: 'svg-inline-loader' },
    ],
  },

  plugins: [new MiniCssExtractPlugin({})],
};

export default [
  {
    ...common,
    target: 'node',
    mode: 'production',
    devtool: 'source-map',
    entry: {
      app: './webapp/lib/index.node.ts',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../dist/lib'),
      libraryTarget: 'commonjs',
      filename: 'lib.node.js',
    },
  },
  {
    ...common,
    target: 'web',
    mode: 'production',
    devtool: 'source-map',
    entry: {
      app: './webapp/lib/index.ts',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../dist/lib'),
      libraryTarget: 'umd',
      library: 'pyroscope',
      filename: 'lib.js',
      globalObject: 'this',
    },

    externals: {
      react: 'react',
      reactDom: 'react-dom',
    },
  },
];
