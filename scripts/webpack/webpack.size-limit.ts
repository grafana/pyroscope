import { merge } from 'webpack-merge';
import path from 'path';
import prod from './webpack.prod';

module.exports = merge(prod, {
  output: {
    publicPath: '',
    path: path.resolve(__dirname, '../../packages/webapp/public/assets'),

    // build WITHOUT hash
    // otherwise size-limit won't know what to compare to
    // https://github.com/andresz1/size-limit-action/issues/47
    filename: '[name].js',
    clean: true,
  },
});
