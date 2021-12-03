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
        let maxTries = 24;
        do {
          if (maxTries <= 0) {
            throw new Error(
              'Could not find pyroscope instance running on http://localhost:4040. Make sure you have pyroscope server running on port :4040'
            );
          }
          res = request('GET', 'http://localhost:4040');
          sleep(1000);
          maxTries -= 1;
        } while (res.statusCode !== 200);

        return res.getBody('utf8');
      },
    }),
  ],
});
