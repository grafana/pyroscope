import { merge } from 'webpack-merge';

import common from './webpack.common';

module.exports = merge(common, {
  mode: 'production',

  // Recommended choice for production builds with high quality SourceMaps.
  devtool: 'source-map',
});
