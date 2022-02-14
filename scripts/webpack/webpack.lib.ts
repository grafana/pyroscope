import path from 'path';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import { ESBuildMinifyPlugin } from 'esbuild-loader';
import ReplaceInFileWebpackPlugin from 'replace-in-file-webpack-plugin';
import CopyWebpackPlugin from 'copy-webpack-plugin';
import { getAlias, getJsLoader, getStyleLoaders } from './shared';

let version = 'dev';
if (process.env.NODE_ENV === 'production') {
  if (!process.env.PYROSCOPE_LIB_VERSION) {
    throw new Error('Environment variable PYROSCOPE_LIB_VERSION is required');
  }
  version = process.env.PYROSCOPE_LIB_VERSION;
}

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

  plugins: [
    new MiniCssExtractPlugin({}),
    new CopyWebpackPlugin({
      patterns: [
        { from: path.join('./webapp/lib/', 'README.md'), to: '.' },
        { from: path.join('./webapp/lib/', 'package.json'), to: '.' },
      ],
    }),
    new ReplaceInFileWebpackPlugin([
      {
        dir: './dist/lib',
        files: ['package.json', 'README.md'],
        rules: [
          {
            search: '%VERSION%',
            replace: version,
          },
          {
            search: '%TODAY%',
            replace: new Date().toISOString().substring(0, 10),
          },
        ],
      },
    ]),
  ],
};

export default [
  {
    ...common,
    target: 'node',
    mode: 'production',
    devtool: 'source-map',
    entry: {
      index: './webapp/lib/index.node.ts',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../dist/lib'),
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
      index: './webapp/lib/index.ts',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../dist/lib'),
      libraryTarget: 'umd',
      library: 'pyroscope',
      filename: 'index.js',
      globalObject: 'this',
    },

    externals: {
      react: 'react',
      reactDom: 'react-dom',
    },
  },
];
