// @ts-nocheck
import webpack from 'webpack';
import path from 'path';
import glob from 'glob';
import fs from 'fs';
import HtmlWebpackPlugin from 'html-webpack-plugin';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import CopyPlugin from 'copy-webpack-plugin';
import { ESBuildMinifyPlugin } from 'esbuild-loader';

import { getAlias, getJsLoader, getStyleLoaders } from './shared';

const packagePath = path.resolve(__dirname, '../../webapp');
const rootPath = path.resolve(__dirname, '../../');

// use a fake hash when running locally
const LOCAL_HASH = 'local';

function getFilename(ext: string) {
  // We may want to produce no hash, example when running size-limit
  if (process.env.NOHASH) {
    return `[name].${ext}`;
  }

  if (process.env.NODE_ENV === 'production') {
    return `[name].[hash].${ext}`;
  }

  // TODO: there's some cache busting issue when running locally
  return `[name].${LOCAL_HASH}.${ext}`;
}

const pages = glob
  .sync(path.join(__dirname, '../../webapp/templates/!(standalone).html'))
  .map((x) => path.basename(x));

const pagePlugins = pages.map(
  (name) =>
    new HtmlWebpackPlugin({
      filename: path.resolve(packagePath, `public/${name}`),
      template: path.resolve(packagePath, `templates/${name}`),
      inject: false,
      templateParameters: (compilation) => {
        // TODO:
        // ideally we should access via argv
        // https://webpack.js.org/configuration/mode/
        const hash =
          process.env.NODE_ENV === 'production'
            ? compilation.getStats().toJson().hash
            : LOCAL_HASH;

        return {
          extra_metadata: process.env.EXTRA_METADATA
            ? fs.readFileSync(process.env.EXTRA_METADATA)
            : '',
          mode: process.env.NODE_ENV,
          webpack: {
            hash,
          },
        };
      },
    })
);

export default {
  target: 'web',

  entry: {
    app: path.join(packagePath, 'javascript/index.tsx'),
    styles: path.join(packagePath, 'sass/profile.scss'),
  },

  output: {
    publicPath: '',
    path: path.resolve(packagePath, 'public/assets'),

    // https://webpack.js.org/guides/build-performance/#avoid-production-specific-tooling
    filename: getFilename('js'),
    clean: true,
  },

  resolve: {
    extensions: ['.ts', '.tsx', '.es6', '.js', '.jsx', '.json', '.svg'],
    alias: getAlias(),
  },

  stats: {
    children: false,
    warningsFilter: /export .* was not found in/,
    source: false,
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
        test: /\.(svg|ico|jpg|jpeg|png|gif|eot|otf|webp|ttf|woff|woff2|cur|ani|pdf)(\?.*)?$/,
        loader: 'file-loader',

        // We output files to assets/static/img, where /assets comes from webpack's output dir
        // However, we still need to prefix the public URL with /assets/static/img
        options: {
          outputPath: 'static/img',
          // using relative path to make this work when pyroscope is deployed to a subpath (with BaseURL config option)
          publicPath: '../assets/static/img',
          name: '[name].[hash:8].[ext]',
        },
      },

      // for SVG used via react
      // we simply inline them as if they were normal react components
      {
        test: /\.svg$/,
        issuer: /\.(j|t)sx?$/,
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
    // uncomment if you want to see the webpack bundle analysis
    // new BundleAnalyzerPlugin(),
    ...pagePlugins,
    new MiniCssExtractPlugin({
      filename: getFilename('css'),
    }),
    new CopyPlugin({
      patterns: [
        {
          from: path.join(packagePath, 'images'),
          to: 'images',
        },
      ],
    }),
  ],
};
