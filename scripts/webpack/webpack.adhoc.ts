import HtmlWebpackPlugin from 'html-webpack-plugin';
import InlineChunkHtmlPlugin from 'inline-chunk-html-plugin';
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
    filename: 'client-bundle.[name].js',
  },
  plugins: [
    new HtmlWebpackPlugin({
      //  inject: true,
      filename: path.resolve(__dirname, `../../webapp/public/adhoc.html`),
      template: path.resolve(__dirname, `../../webapp/templates/adhoc.html`),
    }),
    new InlineChunkHtmlPlugin(HtmlWebpackPlugin, [/client-bundle/]),
  ],
} as any);

export default config;
