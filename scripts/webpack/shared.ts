import MiniCssExtractPlugin from 'mini-css-extract-plugin';

const path = require('path');

// TODO:
export function getStyleLoaders() {
  return [
    {
      test: /\.css$/,
      // include: MONACO_DIR, // https://github.com/react-monaco-editor/react-monaco-editor
      use: ['style-loader', 'css-loader'],
    },
    {
      test: /\.scss$/,
      use: [
        MiniCssExtractPlugin.loader,
        {
          loader: 'css-loader',
          options: {
            importLoaders: 2,
            url: true,
            sourceMap: true,
          },
        },
        {
          loader: 'postcss-loader',
          options: {
            sourceMap: true,
            config: { path: __dirname },
          },
        },
        {
          loader: 'sass-loader',
          options: {
            sourceMap: true,
          },
        },
      ],
    },
  ];
}

export function getAlias() {
  return {
    // rc-trigger uses babel-runtime which has internal dependency to core-js@2
    // this alias maps that dependency to core-js@t3
    'core-js/library/fn': 'core-js/stable',
    '@utils': path.resolve(__dirname, '../../webapp/javascript/util'),
    '@models': path.resolve(__dirname, '../../webapp/javascript/models'),
    '@ui': path.resolve(__dirname, '../../webapp/javascript/ui'),
  };
}

export function getJsLoader() {
  return [
    {
      test: /\.(js|ts)x?$/,
      exclude: /node_modules/,
      use: [
        {
          loader: 'babel-loader',
          options: {
            cacheDirectory: true,
            babelrc: true,

            plugins: ['@babel/plugin-transform-runtime'],
            // Note: order is bottom-to-top and/or right-to-left
            presets: [
              [
                '@babel/preset-env',
                {
                  targets: {
                    browsers: 'last 3 versions',
                  },
                  useBuiltIns: 'entry',
                  corejs: 3,
                  modules: false,
                },
              ],
              [
                '@babel/preset-typescript',
                {
                  allowNamespaces: true,
                },
              ],
              '@babel/preset-react',
            ],
          },
        },
      ],
    },
  ];
}
