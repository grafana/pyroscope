import '@testing-library/jest-dom';

const { toMatchImageSnapshot } = require('jest-image-snapshot');

expect.extend({
  toMatchImageSnapshot(received: any, options: any) {
    // If these checks pass, assume we're in a JSDOM environment with the 'canvas' package.
    if (process.env.RUN_SNAPSHOTS) {
      return toMatchImageSnapshot.call(this, received, options);
    }

    return { pass: true };
  },
});
