import HtmlWebpackPlugin from 'html-webpack-plugin';
import InlineChunkHtmlPlugin from 'react-dev-utils/InlineChunkHtmlPlugin';
import HTMLInlineCSSWebpackPlugin from 'html-inline-css-webpack-plugin';
import path from 'path';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import { getAlias, getJsLoader, getStyleLoaders } from './shared';

// Creates a file in webapp/public/standalone.html
// With js+css+svg embed into the html
const config = (env, options) => {
  let livereload = [];

  // conditionally use require
  // so that in CI when building for production don't have to pull it
  if (options.watch) {
    // eslint-disable-next-line global-require
    const LiveReloadPlugin = require('webpack-livereload-plugin');
    livereload = [
      new LiveReloadPlugin({
        // most likely the default port is used by the main webapp
        port: 35730,
        appendScriptTag: true,
      }),
    ];
  }

  return {
    // We will always run in production mode, even when developing locally
    // reason is that we rely on things like ModuleConcatenation, TerserPlugin etc
    mode: 'production',
    //  devtool: 'eval-source-map',
    entry: {
      app: './webapp/javascript/standalone.tsx',
      styles: './webapp/sass/standalone.scss',
    },

    optimization: {
      mangleExports: false,
      //      minimize: false,
    },

    output: {
      publicPath: '',
      // Emit to another directory other than webapp/public/assets to not have any conflicts
      path: path.resolve(__dirname, '../../dist/standalone'),
      filename: '[name].js',
      clean: true,
    },
    module: {
      // Note: order is bottom-to-top and/or right-to-left
      rules: [
        ...getJsLoader(),
        ...getStyleLoaders(),
        {
          test: /\.svg/,
          use: {
            loader: 'svg-url-loader',
          },
        },
      ],
    },
    resolve: {
      extensions: ['.ts', '.tsx', '.js', '.jsx', '.svg'],
      alias: getAlias(),
      modules: [path.resolve('webapp'), path.resolve('node_modules')],
    },
    plugins: [
      new MiniCssExtractPlugin({
        filename: '[name].[hash].css',
      }),
      new HtmlWebpackPlugin({
        filename: path.resolve(
          __dirname,
          `../../webapp/public/standalone.html`
        ),
        template: path.resolve(
          __dirname,
          `../../webapp/templates/standalone.html`
        ),
      }),
      new InlineChunkHtmlPlugin(HtmlWebpackPlugin, [/.*/]),
      new HTMLInlineCSSWebpackPlugin(),

      ...livereload,
    ],
  };
};

export default config;
