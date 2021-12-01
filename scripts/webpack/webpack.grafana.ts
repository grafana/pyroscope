// ATTENTION
// all this was copied from grafana-toolkit
import * as webpack from 'webpack';
const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const util = require('util');
const CopyWebpackPlugin = require('copy-webpack-plugin');
const ReplaceInFileWebpackPlugin = require('replace-in-file-webpack-plugin');
const ForkTsCheckerWebpackPlugin = require('fork-ts-checker-webpack-plugin');
const TerserPlugin = require('terser-webpack-plugin');
const OptimizeCssAssetsPlugin = require('optimize-css-assets-webpack-plugin');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const fs = require('fs');

let PLUGIN_ID: string;

const readdirPromise = util.promisify(fs.readdir);

const getPluginId = () => {
  if (!PLUGIN_ID) {
    const pluginJson = require(path.resolve(
      process.cwd(),
      'grafana-plugin',
      'src/plugin.json'
    ));
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

const supportedExtensions = ['css', 'scss', 'less', 'sass'];

const getStylesheetPaths = (root: string = process.cwd()) => {
  return [`${root}/src/styles/light`, `${root}/src/styles/dark`];
};

const getCommonPlugins = (options: WebpackConfigurationOptions) => {
  const packageJson = require(path.resolve(process.cwd(), 'package.json'));
  return [
    //new MiniCssExtractPlugin({
    //  // both options are optional
    //  filename: 'styles/[name].css',
    //}),
    new MiniCssExtractPlugin({
      filename: '[name].css',
    }),
    //    new webpack.optimize.OccurrenceOrderPlugin(true),
    //
    new CopyWebpackPlugin({
      patterns: [
        //        // If src/README.md exists use it; otherwise the root README
        {
          from: '../README.md',
          to: '.',
          force: true,
        },
        { from: 'plugin.json', to: '.' },
        //        //        { from: '../LICENSE', to: '.' },
        //        //        { from: '../CHANGELOG.md', to: '.', force: true },
        //        //        { from: '**/*.json', to: '.' },
        //        //        { from: '**/*.svg', to: '.' },
        //        //        { from: '**/*.png', to: '.' },
        //        //        { from: '**/*.html', to: '.' },
        //        //        { from: 'img/**/*', to: '.' },
        //        //        { from: 'libs/**/*', to: '.' },
        //        //        { from: 'static/**/*', to: '.' },
      ],
    }),
    //
    new ReplaceInFileWebpackPlugin([
      {
        dir: 'grafana-plugin/dist',
        files: ['plugin.json', 'README.md'],
        rules: [
          {
            search: '%VERSION%',
            replace: packageJson.version,
          },
          {
            search: '%TODAY%',
            replace: new Date().toISOString().substring(0, 10),
          },
        ],
      },
    ]),
    //
    //    new ForkTsCheckerWebpackPlugin({
    //      typescript: { configFile: path.join(process.cwd(), 'tsconfig.json') },
    //      issue: {
    //        include: [{ file: '**/*.{ts,tsx}' }],
    //      },
    //    }),
  ];
};

const hasThemeStylesheets = (root: string = process.cwd()) => {
  const stylesheetsPaths = getStylesheetPaths(root);
  const stylesheetsSummary: boolean[] = [];

  const result = stylesheetsPaths.reduce((acc, current) => {
    if (fs.existsSync(`${current}.css`) || fs.existsSync(`${current}.scss`)) {
      stylesheetsSummary.push(true);
      return acc && true;
    } else {
      stylesheetsSummary.push(false);
      return false;
    }
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

const getModuleFiles = () => {
  return findModuleFiles(path.resolve(process.cwd(), 'grafana-plugin', 'src'));
};

const findModuleFiles = async (
  base: string,
  files?: string[],
  result?: string[]
) => {
  files = files || (await readdirPromise(base));
  result = result || [];

  if (files) {
    await Promise.all(
      files.map(async (file) => {
        const newbase = path.join(base, file);
        if (fs.statSync(newbase).isDirectory()) {
          result = await findModuleFiles(
            newbase,
            await readdirPromise(newbase),
            result
          );
        } else {
          const filename = path.basename(file);
          if (/^module.(t|j)sx?$/.exec(filename)) {
            // @ts-ignore
            result.push(newbase);
          }
        }
      })
    );
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

const getStyleLoaders = () => {
  const extractionLoader = {
    loader: MiniCssExtractPlugin.loader,
    options: {
      publicPath: '../',
    },
  };

  const cssLoaders2 = [
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

  const cssLoaders = cssLoaders2;
  const styleDir = path.resolve(process.cwd(), 'src', 'styles') + path.sep;
  const rules = [
    {
      test: /(dark|light)\.css$/,
      use: [extractionLoader, ...cssLoaders],
    },
    {
      test: /(dark|light)\.scss$/,
      use: [extractionLoader, ...cssLoaders, 'sass-loader'],
    },
    {
      test: /\.css$/,
      use: ['style-loader', ...cssLoaders, 'sass-loader'],
      exclude: [`${styleDir}light.css`, `${styleDir}dark.css`],
    },
    {
      test: /\.s[ac]ss$/,
      use: ['style-loader', ...cssLoaders, 'sass-loader'],
      exclude: [`${styleDir}light.scss`, `${styleDir}dark.scss`],
    },
    {
      test: /\.less$/,
      use: [
        {
          loader: 'style-loader',
        },
        ...cssLoaders,
        {
          loader: 'less-loader',
          options: {
            javascriptEnabled: true,
          },
        },
      ],
      exclude: [`${styleDir}light.less`, `${styleDir}dark.less`],
    },
  ];

  return cssLoaders2;
  //  return rules;
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
    //    node: {
    //      fs: 'empty',
    //      net: 'empty',
    //      tls: 'empty',
    //    },
    context: path.join(process.cwd(), 'grafana-plugin', 'src'),
    devtool: 'source-map',
    entry: {
      module: path.join(process.cwd(), 'grafana-plugin', 'src', 'module.ts'),
    },
    output: {
      filename: '[name].js',
      path: path.join(process.cwd(), 'grafana-plugin', 'dist'),
      libraryTarget: 'amd',
      publicPath: '/',
    },

    performance: { hints: false },
    externals: [
      'tslib',
      'lodash',
      'jquery',
      'moment',
      'slate',
      'emotion',
      '@emotion/react',
      '@emotion/css',
      'prismjs',
      'slate-plain-serializer',
      '@grafana/slate-react',
      'react',
      'react-dom',
      'react-redux',
      'redux',
      'rxjs',
      'react-router-dom',
      'd3',
      'angular',
      '@grafana/ui',
      '@grafana/runtime',
      '@grafana/data',
      // @ts-ignore
      (context, request, callback) => {
        const prefix = 'grafana/';
        if (request.indexOf(prefix) === 0) {
          return callback(null, request.substr(prefix.length));
        }

        // @ts-ignore
        callback();
      },
    ],
    plugins,
    resolve: {
      extensions: ['.ts', '.tsx', '.js', '.jsx'],
      modules: [path.resolve(process.cwd(), 'src'), 'node_modules'],
      alias: {
        // rc-trigger uses babel-runtime which has internal dependency to core-js@2
        // this alias maps that dependency to core-js@t3
        'core-js/library/fn': 'core-js/stable',
        '@utils': path.resolve(__dirname, '../../webapp/javascript/util'),
        '@models': path.resolve(__dirname, '../../webapp/javascript/models'),
        '@ui': path.resolve(__dirname, '../../webapp/javascript/ui'),
      },
      fallback: {
        fs: false,
        net: false,
        tls: false,
      },
    },
    module: {
      rules: [
        {
          test: /\.(js|ts)x?$/,
          exclude: /node_modules/,
          use: [
            {
              loader: 'babel-loader',
              options: {
                cacheDirectory: true,
                babelrc: true,
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
        //        {
        //          test: /\.(js|ts)x?$/,
        //          //  test: /\.tsx?$/,
        //          use: [
        //            {
        //              loader: require.resolve('babel-loader'),
        //              options: {
        //                presets: [
        //                  [require.resolve('@babel/preset-env'), { modules: false }],
        //                ],
        //                //                plugins: [require.resolve('babel-plugin-angularjs-annotate')],
        //                plugins: [],
        //                sourceMaps: true,
        //              },
        //            },
        //            //   {
        //            //     //  loader: require.resolve('ts-loader'),
        //            //     loader: 'babel-loader',
        //            //     options: {
        //            //       onlyCompileBundledFiles: true,
        //            //       transpileOnly: true,
        //            //     },
        //            //   },
        //          ],
        //          exclude: /(node_modules)/,
        //        },
        //        {
        //          test: /\.jsx?$/,
        //          loaders: [
        //            {
        //              loader: require.resolve('babel-loader'),
        //              options: {
        //                presets: [['@babel/preset-env', { modules: false }]],
        //                plugins: ['angularjs-annotate'],
        //                sourceMaps: true,
        //              },
        //            },
        //          ],
        //          exclude: /(node_modules)/,
        //        },
        ...getStyleLoaders(),
        //        {
        //          test: /\.html$/,
        //          exclude: [/node_modules/],
        //          use: {
        //            loader: require.resolve('html-loader'),
        //          },
        //        },
        ...getFileLoaders(),
      ],
    },
    optimization,
  };
};

const accessPromise = util.promisify(fs.access);

const loadWebpackConfig: WebpackConfigurationGetter = async (options) => {
  const baseConfig = await getBaseWebpackConfig(options);
  const customWebpackPath = path.resolve(process.cwd(), 'webpack.config.js');

  try {
    await accessPromise(customWebpackPath);
    const customConfig = require(customWebpackPath);
    const configGetter = customConfig.getWebpackConfig || customConfig;
    if (typeof configGetter !== 'function') {
      throw Error(
        'Custom webpack config needs to export a function implementing CustomWebpackConfigurationGetter. Function needs to be ' +
          'module export or named "getWebpackConfig"'
      );
    }
    return (configGetter as CustomWebpackConfigurationGetter)(
      baseConfig,
      options
    );
  } catch (err: any) {
    if (err.code === 'ENOENT') {
      return baseConfig;
    }
    throw err;
  }
};

//loadWebpackConfig({}).then((a) => console.log(a));
module.exports = loadWebpackConfig({});
