import HtmlWebpackPlugin from 'html-webpack-plugin';
import InlineChunkHtmlPlugin from 'react-dev-utils/InlineChunkHtmlPlugin';
import HTMLInlineCSSWebpackPlugin from 'html-inline-css-webpack-plugin';
import path from 'path';
import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import { getAlias, getJsLoader, getStyleLoaders } from './shared';

const config = {
  mode: 'production',
  entry: {
    app: './webapp/javascript/adhoc.tsx',
    styles: './webapp/sass/standalone.scss',
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
      filename: path.resolve(__dirname, `../../webapp/public/adhoc.html`),
      template: path.resolve(__dirname, `../../webapp/templates/adhoc.html`),
    }),
    new InlineChunkHtmlPlugin(HtmlWebpackPlugin, [/.*/]),
    new HTMLInlineCSSWebpackPlugin(),
  ],
};

export default config;
