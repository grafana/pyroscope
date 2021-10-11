const { merge } = require('webpack-merge');
const path = require('path');
const prod = require('./webpack.prod');

module.exports = merge(prod, {
  output: {
    publicPath: '',
    path: path.resolve(__dirname, '../../webapp/public/assets'),

    // build WITHOUT hash
    // otherwise size-limit won't know what to compare to
    //
    // https://github.com/andresz1/size-limit-action/issues/47
    filename: '[name].js',
    clean: true,
  },
});
