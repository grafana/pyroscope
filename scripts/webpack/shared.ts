import MiniCssExtractPlugin from 'mini-css-extract-plugin';
import path from 'path';

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
    '@pyroscope/redux': path.resolve(
      __dirname,
      '../../webapp/javascript/redux'
    ),
    '@pyroscope/services': path.resolve(
      __dirname,
      '../../webapp/javascript/services'
    ),
  };
}

export function getJsLoader() {
  return [
    {
      test: /\.(js|ts)x?$/,
      exclude: /node_modules/,
      use: [
        {
          loader: 'esbuild-loader',
          options: {
            loader: 'tsx', // Or 'ts' if you don't need tsx
            target: 'esnext',
          },
        },
      ],
    },
  ];
}
