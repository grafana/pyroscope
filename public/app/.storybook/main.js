const path = require('path');

module.exports = {
  stories: [
    '../stories/**/*.stories.mdx',
    '../stories/**/*.stories.@(js|jsx|ts|tsx)',
  ],
  addons: ['@storybook/addon-links', '@storybook/addon-essentials'],
  core: {
    builder: 'webpack5',
  },
  webpackFinal: async (config) => {
    config.resolve.alias = {
      ...config.resolve.alias,
      // Only allow importing ui elements, since at some point we want to move
      // ui to its own package
      '@pyroscope/ui': path.resolve(__dirname, '../ui'),
      '@ui': path.resolve(__dirname, '..//ui'),

      '@utils': path.resolve(__dirname, '../util'),
    };
    config.resolve.extensions.push('.ts', '.tsx');

    // support sass
    config.module.rules.push({
      test: /\.scss$/,
      use: ['style-loader', 'css-loader', 'sass-loader'],
      include: path.resolve(__dirname, '../'),
    });

    // https://github.com/storybookjs/storybook/issues/6188#issuecomment-1026502543
    // remove svg from existing rule
    config.module.rules = config.module.rules.map((rule) => {
      if (
        String(rule.test) ===
        String(
          /\.(svg|ico|jpg|jpeg|png|apng|gif|eot|otf|webp|ttf|woff|woff2|cur|ani|pdf)(\?.*)?$/
        )
      ) {
        return {
          ...rule,
          test: /\.(ico|jpg|jpeg|png|apng|gif|eot|otf|webp|ttf|woff|woff2|cur|ani|pdf)(\?.*)?$/,
        };
      }

      return rule;
    });

    // This was copied from our main webpack config
    config.module.rules.push({
      test: /\.svg$/,
      use: [
        {
          loader: 'react-svg-loader',
          options: {
            svgo: {
              plugins: [
                { convertPathData: { noSpaceAfterFlags: false } },
                { removeViewBox: false },
              ],
            },
          },
        },
      ],
    });
    return config;
  },
};
