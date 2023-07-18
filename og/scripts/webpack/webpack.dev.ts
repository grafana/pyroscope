import { merge } from 'webpack-merge';
import LiveReloadPlugin from 'webpack-livereload-plugin';
import common from './webpack.common';

module.exports = merge(common, {
  devtool: 'eval-source-map',
  mode: 'development',
  plugins: [
    new LiveReloadPlugin({
      appendScriptTag: true,
    }),
  ],
  // TODO deal with these types
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
} as any);
