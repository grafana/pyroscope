import '@testing-library/jest-dom';
import 'jest-canvas-mock';
import type { MatchImageSnapshotOptions } from 'jest-image-snapshot';
import nodeFetch from 'node-fetch';
globalThis.fetch = nodeFetch as unknown as typeof fetch;

async function getToMatchImageSnapshot() {
  if (!process.env.RUN_SNAPSHOTS) {
    return () => ({
      pass: true,
      message: () =>
        `Skipping 'toMatchImageSnapshot' assertion since env var 'RUN_SNAPSHOTS' is not set.`,
    });
  }

  return (await import('jest-image-snapshot')).configureToMatchImageSnapshot({
    // Big enough threshold to account for different font rendering
    // TODO: fix it
    failureThreshold: 0.1,
    failureThresholdType: 'percent',
  });
}

async function setupExt() {
  const toMatchImageSnapshot = await getToMatchImageSnapshot();

  expect.extend({
    toMatchImageSnapshot(received: string, options: MatchImageSnapshotOptions) {
      // If these checks pass, assume we're in a JSDOM environment with the 'canvas' package.
      return toMatchImageSnapshot.call(this, received, options);
    },
  });
}

setupExt();
