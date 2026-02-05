const CopyWebpackPlugin = require('copy-webpack-plugin');

const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');

module.exports = {
  target: 'web',
  entry: {
    app: './public/app/app.tsx',
    admin: './public/app/admin/admin.tsx',
  },
  output: {
    clean: true,
    path: path.resolve(__dirname, '../../public/build'),
    filename: (pathData) => {
      // Use fixed filename for admin bundle (served from Go template)
      // Use content hash for app bundle (injected by HtmlWebpackPlugin)
      return pathData.chunk.name === 'admin'
        ? '[name].js'
        : '[name].[contenthash].js';
    },
    publicPath: '',
  },
  resolve: {
    extensions: ['.ts', '.tsx', '.es6', '.js', '.json', '.svg', '.ttf'],
    modules: ['node_modules', path.resolve('public')],

    // TODO: unify with tsconfig.json
    // When using TsconfigPathsPlugin, paths overrides don't work
    // For example, setting a) '@webapp/*' and  b) '@webapp/components/ExportData'
    // Would end up ignoring b)
    alias: {
      '@pyroscope': path.resolve(__dirname, '../../public/app'),
      // some sub-dependencies use a different version of @emotion/react and generate warnings
      // in the browser about @emotion/react loaded twice. We want to only load it once
      '@emotion/react': require.resolve('@emotion/react'),
      // Redirect react-use CJS imports to ESM build for proper module interop
      // @grafana/ui and @grafana/flamegraph import from react-use/lib/* which causes ESM/CJS issues
      'react-use/lib/useAsync': 'react-use/esm/useAsync',
      'react-use/lib/useClickAway': 'react-use/esm/useClickAway',
      'react-use/lib/useDebounce': 'react-use/esm/useDebounce',
      'react-use/lib/useMeasure': 'react-use/esm/useMeasure',
      'react-use/lib/usePrevious': 'react-use/esm/usePrevious',
      // Fix CommonJS/ESM interop for react-custom-scrollbars-2 ($ = exact match only)
      'react-custom-scrollbars-2$': path.resolve(
        __dirname,
        './stubs/react-custom-scrollbars-2.js'
      ),
      // Dependencies
      //...deps,
    },
    plugins: [
      // Use same alias from tsconfig.json
      //      new TsconfigPathsPlugin({
      //        logLevel: 'info',
      //        // TODO: figure out why it could not use the baseUrl from tsconfig.json
      //        baseUrl: path.resolve(__dirname, '../../'),
      //      }),
    ],
  },
  ignoreWarnings: [/export .* was not found in/],
  stats: {
    children: false,
    source: false,
  },
  plugins: [
    new MiniCssExtractPlugin({
      filename: (pathData) => {
        // Use fixed filename for admin bundle (served from Go template)
        // Use content hash for app bundle (injected by HtmlWebpackPlugin)
        return pathData.chunk.name === 'admin'
          ? '[name].css'
          : '[name].[contenthash].css';
      },
    }),
    new CopyWebpackPlugin({
      patterns: [
        {
          from: 'node_modules/@grafana/ui/dist/public/img/icons',
          to: 'grafana/build/img/icons/',
        },
      ],
    }),
  ],
  module: {
    rules: [
      // Fix for ESM modules in node_modules that don't include file extensions
      // This is required for @grafana/flamegraph and its dependencies (rc-picker, ol, etc.)
      {
        test: /\.m?js$/,
        resolve: {
          fullySpecified: false,
        },
      },
      // CSS
      {
        test: /\.(css|scss)$/,
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
            loader: 'sass-loader',
            options: {},
          },
        ],
      },

      {
        test: require.resolve('jquery'),
        loader: 'expose-loader',
        options: {
          exposes: ['$', 'jQuery'],
        },
      },
      {
        test: /\.(js|ts)x?$/,
        // Ignore everything except pyroscope-oss, since it's used as if it was local code
        // exclude: /node_modules\/(?!pyroscope-oss).*/,
        use: [
          {
            loader: 'esbuild-loader',
            options: {
              loader: 'tsx',
              target: 'es2015',
            },
          },
        ],
      },

      // SVG
      {
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
      },
      {
        test: /\.ttf$/,
        type: 'asset/resource',
      },
    ],
  },
};
