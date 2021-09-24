/// <reference types="cypress" />
// ***********************************************************
// This example plugins/index.js can be used to load plugins
//
// You can change the location of this file or turn off loading
// the plugins file with the 'pluginsFile' configuration option.
//
// You can read more here:
// https://on.cypress.io/plugins-guide
// ***********************************************************

// This function is called when a project is opened or re-opened (e.g. due to
// the project's config changing)
const { initPlugin } = require('cypress-plugin-snapshots/plugin');
const {
  addMatchImageSnapshotPlugin,
} = require('cypress-image-snapshot/plugin');

/**
 * @type {Cypress.PluginConfig}
 */
// eslint-disable-next-line no-unused-vars
module.exports = (on, config) => {
  // `on` is used to hook into various events Cypress emits
  // `config` is the resolved Cypress config
  //
  initPlugin(on, config);
  addMatchImageSnapshotPlugin(on, config);

  // https://github.com/cypress-io/cypress/issues/587#issuecomment-812115782
  // https://docs.cypress.io/api/plugins/browser-launch-api#Examples
  on('before:browser:launch', (browser, launchOptions) => {
    if (browser.name === 'chrome' && browser.isHeadless) {
      // fullPage screenshot size is 1400x1200 on non-retina screens
      // and 2800x2400 on retina screens
      launchOptions.args.push('--window-size=1400,1200');

      // force screen to be non-retina (1400x1200 size)
      launchOptions.args.push('--force-device-scale-factor=1');

      // force screen to be retina (2800x2400 size)
      // launchOptions.args.push('--force-device-scale-factor=2')
    }

    if (browser.name === 'electron' && browser.isHeadless) {
      // fullPage screenshot size is 1400x1200
      launchOptions.preferences.width = 1400;
      launchOptions.preferences.height = 1200;
    }

    if (browser.name === 'firefox' && browser.isHeadless) {
      // menubars take up height on the screen
      // so fullPage screenshot size is 1400x1126
      launchOptions.args.push('--width=1400');
      launchOptions.args.push('--height=1200');
    }

    return launchOptions;
  });

  return config;
};
