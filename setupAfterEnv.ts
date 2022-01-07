import '@testing-library/jest-dom';
import 'jest-canvas-mock';
import timezoneMock from 'timezone-mock';
import { configureToMatchImageSnapshot } from 'jest-image-snapshot';
import type { MatchImageSnapshotOptions } from 'jest-image-snapshot';
import nodeFetch from 'node-fetch';

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

    // This is running in node
    // eslint-disable-next-line no-console
    //    console.info(
    //      `Skipping 'toMatchImageSnapshot' assertion since env var 'RUN_SNAPSHOTS' is not set.`
    //    );

    return {
      pass: true,
      message: () =>
        `Skipping 'toMatchImageSnapshot' assertion since env var 'RUN_SNAPSHOTS' is not set.`,
    };
  },
});

timezoneMock.register('UTC');

globalThis.fetch = nodeFetch as unknown as typeof fetch;

// When testing redux we can assume this will be populated
// Which will be used for setting up the initialState
(globalThis.window as any).initialState = {
  appNames: [],
};
