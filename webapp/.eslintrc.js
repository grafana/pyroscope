const path = require('path');

module.exports = {
  extends: [path.join(__dirname, '../.eslintrc.js')],
  ignorePatterns: ['public', 'javascript/util', '*.spec.*', '.eslintrc.js'],

  rules: {
    // https://github.com/import-js/eslint-plugin-import/issues/1174
    'import/no-extraneous-dependencies': 'off',
    // since we use immutablejs in the reducer
    'no-param-reassign': 'off',
  },
};
