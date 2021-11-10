import '@testing-library/jest-dom';
import 'jest-canvas-mock';

const {
  toMatchImageSnapshot,
  configureToMatchImageSnapshot,
} = require('jest-image-snapshot');

expect.extend({
  toMatchImageSnapshot(received: any, options: any) {
    // If these checks pass, assume we're in a JSDOM environment with the 'canvas' package.
    if (process.env.RUN_SNAPSHOTS) {
      const customConfig = { threshold: 0.02 };
      const toMatchImageSnapshot = configureToMatchImageSnapshot({
        customDiffConfig: customConfig,
      });

      return toMatchImageSnapshot.call(this, received, options);
    }

    console.info(
      `Skipping 'toMatchImageSnapshot' assertion since env var 'RUN_SNAPSHOTS' is not set.`
    );
    return { pass: true };
  },
});
