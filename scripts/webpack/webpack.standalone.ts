import HtmlWebpackPlugin from 'html-webpack-plugin';
import InlineChunkHtmlPlugin from 'react-dev-utils/InlineChunkHtmlPlugin';
import HTMLInlineCSSWebpackPlugin from 'html-inline-css-webpack-plugin';
import path from 'path';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import LiveReloadPlugin from 'webpack-livereload-plugin';
import { getAlias, getJsLoader, getStyleLoaders } from './shared';

// Creates a file in webapp/public/standalone.html
// With js+css+svg embed into the html
const config = (env, options) => {
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

      ...(options.watch
        ? [
            new LiveReloadPlugin({
              appendScriptTag: true,
            }),
          ]
        : []),
    ],
  };
};

export default config;
