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
          },
        },
        {
          loader: 'postcss-loader',
          options: {
            config: { path: __dirname },
          },
        },
        {
          loader: 'sass-loader',
          options: {
            //           sourceMap: true,
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
    '@webapp': path.resolve(__dirname, '../../webapp/javascript'),
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
            target: 'es2015',
          },
        },
        //        {
        //          loader: 'babel-loader',
        //          options: {
        //            cacheDirectory: true,
        //            babelrc: true,
        //
        //            plugins: ['@babel/plugin-transform-runtime'],
        //            // Note: order is bottom-to-top and/or right-to-left
        //            presets: [
        //              [
        //                '@babel/preset-env',
        //                {
        //                  targets: {
        //                    browsers: 'last 3 versions',
        //                  },
        //                  useBuiltIns: 'entry',
        //                  corejs: 3,
        //                  modules: false,
        //                },
        //              ],
        //              [
        //                '@babel/preset-typescript',
        //                {
        //                  allowNamespaces: true,
        //                },
        //              ],
        //              '@babel/preset-react',
        //            ],
        //          },
        //        },
      ],
    },
  ];
}
