import HtmlWebpackPlugin from 'html-webpack-plugin';
import InlineChunkHtmlPlugin from 'react-dev-utils/InlineChunkHtmlPlugin';
import HTMLInlineCSSWebpackPlugin from 'html-inline-css-webpack-plugin';
import path from 'path';
import { merge } from 'webpack-merge';

import common from './webpack.common';

const config = merge(common, {
  mode: 'production',
  entry: {
    app: './webapp/javascript/adhoc.tsx',
    styles: './webapp/sass/profile.scss',
  },

  output: {
    // Emit to another directory other than webapp/public/assets to not have any conflicts
    path: path.resolve(__dirname, '../../dist/standalone'),
    filename: '[name].js',
  },
  plugins: [
    new HtmlWebpackPlugin({
      filename: path.resolve(__dirname, `../../webapp/public/adhoc.html`),
      template: path.resolve(__dirname, `../../webapp/templates/adhoc.html`),
    }),
    new InlineChunkHtmlPlugin(HtmlWebpackPlugin, [/.*/]),
    new HTMLInlineCSSWebpackPlugin(),
  ],
} as any);

export default config;
