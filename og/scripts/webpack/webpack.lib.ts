import path from 'path';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import { ESBuildMinifyPlugin } from 'esbuild-loader';
import CopyWebpackPlugin from 'copy-webpack-plugin';
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
    new CopyWebpackPlugin({
      patterns: [
        { from: path.join('./packages/flamegraph/', 'README.md'), to: '.' },
        {
          from: path.join('./packages/flamegraph/', 'package.json'),
          to: '.',
        },
      ],
    }),
    //   new ReplaceInFileWebpackPlugin([
    //     {
    //       dir: './dist/lib',
    //       files: ['package.json', 'README.md'],
    //       rules: [
    //         {
    //           search: '%VERSION%',
    //           replace: version,
    //         },
    //         {
    //           search: '%TODAY%',
    //           replace: new Date().toISOString().substring(0, 10),
    //         },
    //       ],
    //     },
    //   ]),
  ],
};

export default [
  {
    ...common,
    target: 'node',
    mode: 'production',
    devtool: 'source-map',
    entry: {
      index: './packages/flamegraph/src/index.node.ts',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../packages/flamegraph/dist'),
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
      index: './packages/flamegraph/src/index.tsx',
    },
    output: {
      publicPath: '',
      path: path.resolve(__dirname, '../../packages/flamegraph/dist'),
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
