/* eslint-disable */
// ATTENTION
// all this was copied from grafana-toolkit
import * as webpack from 'webpack';

const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const CopyWebpackPlugin = require('copy-webpack-plugin');
const ReplaceInFileWebpackPlugin = require('replace-in-file-webpack-plugin');
const TerserPlugin = require('terser-webpack-plugin');
const OptimizeCssAssetsPlugin = require('optimize-css-assets-webpack-plugin');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const fs = require('fs');
import {
  getStyleLoaders as getCommonStyles,
  getAlias,
  getJsLoader,
} from './shared';

let PLUGIN_ID: string;

const pluginPath = path.join(
  __dirname,
  '../../packages/pyroscope-datasource-plugin'
);

const getPluginId = () => {
  if (!PLUGIN_ID) {
    const pluginJson = require(path.resolve(pluginPath, 'plugin.json'));
    PLUGIN_ID = pluginJson.id;
  }
  return PLUGIN_ID;
};

interface WebpackConfigurationOptions {
  watch?: boolean;
  production?: boolean;
  preserveConsole?: boolean;
}
export type CustomWebpackConfigurationGetter = (
  originalConfig: webpack.Configuration,
  options: WebpackConfigurationOptions
) => webpack.Configuration;

type WebpackConfigurationGetter = (
  options: WebpackConfigurationOptions
) => Promise<webpack.Configuration>;

const getStylesheetPaths = (root: string = process.cwd()) => {
  return [`${root}/src/styles/light`, `${root}/src/styles/dark`];
};

const getCommonPlugins = (options: WebpackConfigurationOptions) => {
  const packageJson = require(path.resolve(process.cwd(), 'package.json'));
  let version = 'dev';
  if (process.env.NODE_ENV === 'production') {
    version = packageJson.version;
  }

  return [
    new MiniCssExtractPlugin({
      filename: '[name].css',
    }),
    new CopyWebpackPlugin({
      patterns: [
        { from: 'README.md', to: '.' },
        { from: 'plugin.json', to: '.' },
        { from: 'CHANGELOG.md', to: '.' },
        { from: 'LICENSE', to: '.' },
        { from: 'img/**/*', to: '.' },
      ],
    }),
    new ReplaceInFileWebpackPlugin([
      {
        dir: path.join(pluginPath, 'dist'),
        files: ['plugin.json', 'README.md'],
        rules: [
          {
            search: '%VERSION%',
            replace: version,
          },
          {
            search: '%TODAY%',
            replace: new Date().toISOString().substring(0, 10),
          },
        ],
      },
    ]),
  ];
};

const hasThemeStylesheets = (root: string = process.cwd()) => {
  const stylesheetsPaths = getStylesheetPaths(root);
  const stylesheetsSummary: boolean[] = [];

  const result = stylesheetsPaths.reduce((acc, current) => {
    if (fs.existsSync(`${current}.css`) || fs.existsSync(`${current}.scss`)) {
      stylesheetsSummary.push(true);
      return acc && true;
    }
    stylesheetsSummary.push(false);
    return false;
  }, true);

  const hasMissingStylesheets =
    stylesheetsSummary.filter((s) => s).length === 1;

  // seems like there is one theme file defined only
  if (result === false && hasMissingStylesheets) {
    console.error(
      '\nWe think you want to specify theme stylesheet, but it seems like there is something missing...'
    );
    stylesheetsSummary.forEach((s, i) => {
      if (s) {
        console.log(stylesheetsPaths[i], 'discovered');
      } else {
        console.log(stylesheetsPaths[i], 'missing');
      }
    });

    throw new Error('Stylesheet missing!');
  }

  return result;
};

const getFileLoaders = () => {
  const shouldExtractCss = hasThemeStylesheets();

  return [
    {
      test: /\.(png|jpe?g|gif|svg)$/,
      use: [
        shouldExtractCss
          ? {
              loader: require.resolve('file-loader'),
              options: {
                outputPath: '/',
                name: '[path][name].[ext]',
              },
            }
          : // When using single css import images are inlined as base64 URIs in the result bundle
            {
              loader: 'url-loader',
            },
      ],
    },
    {
      test: /\.(woff|woff2|eot|ttf|otf)(\?v=\d+\.\d+\.\d+)?$/,
      loader: require.resolve('file-loader'),
      options: {
        // Keep publicPath relative for host.com/grafana/ deployments
        publicPath: `public/plugins/${getPluginId()}/fonts`,
        outputPath: 'fonts',
        name: '[name].[ext]',
      },
    },
  ];
};

const getBaseWebpackConfig: any = async (options) => {
  const plugins = getCommonPlugins(options);
  const optimization: { [key: string]: any } = {};

  if (options.production) {
    const compressOptions = {
      drop_console: !options.preserveConsole,
      drop_debugger: true,
    };
    optimization.minimizer = [
      new TerserPlugin({
        sourceMap: true,
        terserOptions: { compress: compressOptions },
      }),
      new OptimizeCssAssetsPlugin(),
    ];
  } else if (options.watch) {
    plugins.push(new HtmlWebpackPlugin());
  }

  return {
    mode: options.production ? 'production' : 'development',
    target: 'web',
    context: pluginPath,
    devtool: 'source-map',
    entry: {
      module: path.join(pluginPath, 'src', 'module.ts'),
    },
    output: {
      filename: '[name].js',
      path: path.join(pluginPath, 'dist'),
      libraryTarget: 'amd',
      publicPath: '/',
    },

    performance: { hints: false },
    externals: [
      '@grafana/slate-react',
      'react',
      'react-dom',
      'react-redux',
      'redux',
      'react-router-dom',
      '@grafana/ui',
      '@grafana/runtime',
      '@grafana/data',
    ],
    plugins,
    resolve: {
      extensions: ['.ts', '.tsx', '.js', '.jsx'],
      modules: [path.resolve(process.cwd(), 'src'), 'node_modules'],
      alias: getAlias(),
      fallback: {
        fs: false,
        net: false,
        tls: false,
      },
    },
    module: {
      rules: [...getJsLoader(), ...getCommonStyles(), ...getFileLoaders()],
    },
    optimization,
  };
};

const loadWebpackConfig: WebpackConfigurationGetter = async (options) => {
  const baseConfig = await getBaseWebpackConfig(options);

  return baseConfig;
};

module.exports = loadWebpackConfig({});
