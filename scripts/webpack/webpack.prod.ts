// @ts-nocheck
import { merge } from 'webpack-merge';

import common from './webpack.common';

export default merge(common, {
  mode: 'production',

  // Recommended choice for production builds with high quality SourceMaps.
  devtool: 'source-map',
  // TODO deal with these types
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
} as any);
