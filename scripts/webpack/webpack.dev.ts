const { merge } = require('webpack-merge');
const { WebpackPluginServe } = require('webpack-plugin-serve');
const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const request = require('sync-request');
const fs = require('fs');
const route = require('koa-route');

const common = require('./webpack.common');

const sleep = (ms) => new Promise((res) => setTimeout(res, ms));

module.exports = merge(common, {
  devtool: 'eval-source-map',
  mode: 'development',
  entry: {
    serve: 'webpack-plugin-serve/client',
  },

  plugins: [
    // create a server on port 4041 with live reload
    // it will serve all static assets com webapp/public/assets
    // and for the endpoints it will redirect to the go server (on port 4040)
    new WebpackPluginServe({
      port: 4041,
      static: path.resolve(__dirname, '../../webapp/public'),
      liveReload: true,
      waitForBuild: true,
      middleware: (app, builtins) => {
        // TODO
        // this sucks, maybe update endpoints to prefix with /api?
        app.use(builtins.proxy('/render', { target: 'http://localhost:4040' }));
        app.use(
          builtins.proxy('/render-diff', { target: 'http://localhost:4040' })
        );
        app.use(builtins.proxy('/labels', { target: 'http://localhost:4040' }));
        app.use(
          builtins.proxy('/labels-diff', { target: 'http://localhost:4040' })
        );

        // serve index for all pages
        // that are not static (.css, .js) nor live reload (/wps)
        // TODO: simplify this
        app.use(
          route.get(/^(.(?!(\.js|\.css|\.svg|wps)$))+$/, (ctx) => {
            ctx.body = fs.readFileSync(
              path.resolve(__dirname, '../../webapp/public/assets/index.html'),
              {
                encoding: 'utf-8',
              }
            );
          })
        );
      },
    }),

    // serve index.html from the go server
    // and additionally inject anything else required (eg livereload ws)
    new HtmlWebpackPlugin({
      publicPath: '/assets',
      templateContent: () => {
        let res;

        // TODO: accept this to be overwritten?
        // that's useful for when running on a different port (when you are running multiple pyroscope versions locally)
        // or when running on ipv6
        const goServerAddr = 'http://localhost:4040';

        try {
          console.log(`Trying to access go server on ${goServerAddr}`);

          // makes a request against the go server to retrieve its index.html
          // it assumes the server will either not respond or respond with 2xx
          // (ie it doesn't handle != 2xx status codes)
          // https://www.npmjs.com/package/sync-request
          res = request('GET', goServerAddr, {
            timeout: 1000,
            maxRetries: 30,
            retryDelay: 1000,
            retry: true,
          });
        } catch (e) {
          throw new Error(
            `Could not find pyroscope instance running on ${goServerAddr}. Make sure you have pyroscope server running on port :4040`
          );
        }

        console.log('Live reload server is up');

        return res.getBody('utf8');
      },
    }),
  ],
});
