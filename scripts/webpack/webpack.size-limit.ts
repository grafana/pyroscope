import { merge } from 'webpack-merge';
import path from 'path';
import prod from './webpack.prod';

module.exports = merge(prod, {
  output: {
    publicPath: '',
    path: path.resolve(__dirname, '../../webapp/public/assets'),
    clean: true,
  },
});
