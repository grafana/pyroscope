import { merge } from 'webpack-merge';

import common from './webpack.common';

export default merge(common, {
  mode: 'production',

  // Recommended choice for production builds with high quality SourceMaps.
  devtool: 'source-map',
} as any);
// TODO deal with these types
