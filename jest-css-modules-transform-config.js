const path = require('path');

module.exports = {
  sassConfig: {
    // So that we can import scss files using ~mymodule
    // https://github.com/Connormiha/jest-css-modules-transform/issues/32#issuecomment-787437223
    importer: [
      (url, prev) => {
        if (!url.startsWith('~')) return null;

        return {
          file: path.join(__dirname, `node_modules/${url.slice(1)}`),
        };
      },
    ],
  },
};
