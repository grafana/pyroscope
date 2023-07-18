import '@testing-library/jest-dom';
import 'jest-canvas-mock';
import { configureToMatchImageSnapshot } from 'jest-image-snapshot';
import type { MatchImageSnapshotOptions } from 'jest-image-snapshot';
import 'regenerator-runtime/runtime';

// TODO: maybe we don't need this file?
expect.extend({
  toMatchImageSnapshot(received: string, options: MatchImageSnapshotOptions) {
    // If these checks pass, assume we're in a JSDOM environment with the 'canvas' package.
    if (process.env.RUN_SNAPSHOTS) {
      const customConfig = { threshold: 0.02 };
      const toMatchImageSnapshot = configureToMatchImageSnapshot({
        customDiffConfig: customConfig,
      }) as any;

      // TODO
      // for some reason it fails with
      // Expected 1 arguments, but got 3.
      // hence the any
      return toMatchImageSnapshot.call(this, received, options);
    }

    return {
      pass: true,
      message: () =>
        `Skipping 'toMatchImageSnapshot' assertion since env var 'RUN_SNAPSHOTS' is not set.`,
    };
  },
});
