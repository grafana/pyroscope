const path = require('path');

module.exports = {
  extends: [path.join(__dirname, '../../.eslintrc.js')],
  ignorePatterns: ['.eslintrc.js'],
};
