// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import CanvasConverter from 'canvas-to-buffer';
import { createCanvas } from 'canvas';
import { Maybe } from 'true-myth';
import TestData from './testData';
import Flamegraph from './Flamegraph';
import { DefaultPalette } from './colorPalette';
import { configureToMatchImageSnapshot } from 'jest-image-snapshot';
import type { MatchImageSnapshotOptions } from 'jest-image-snapshot';

type focusedNodeType = ConstructorParameters<typeof Flamegraph>[2];
type zoomType = ConstructorParameters<typeof Flamegraph>[5];

expect.extend({
  toMatchImageSnapshot(received: string, options: MatchImageSnapshotOptions) {
    // If these checks pass, assume we're in a JSDOM environment with the 'canvas' package.
    if (process.env.RUN_SNAPSHOTS) {
      const toMatchImageSnapshot = configureToMatchImageSnapshot({
        // Big enough threshold to account for different font rendering
        // TODO: fix it
        failureThreshold: 0.1,
        failureThresholdType: 'percent',
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
// All tests here refer strictly to the rendering bit of "Flamegraph"
describe("render group:snapshot'", () => {
  // TODO i'm thinking here if we can simply reuse this?
  const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;
  const fitMode = 'HEAD';
  const highlightQuery = '';
  const zoom: zoomType = Maybe.nothing();
  const focusedNode: focusedNodeType = Maybe.nothing();

  it('renders a simple flamegraph', () => {
    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom,
      DefaultPalette
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  // this test servers to validate funcionality like collapsing nodes
  it('renders a complex flamegraph', () => {
    const flame = new Flamegraph(
      TestData.ComplexTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom,
      DefaultPalette
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a double(diff) flamegraph', () => {
    const flame = new Flamegraph(
      TestData.DiffTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom,
      DefaultPalette
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a highlighted flamegraph', () => {
    const highlightQuery = 'main';
    const focusedNode: focusedNodeType = Maybe.nothing();

    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom,
      DefaultPalette
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a highlighted double flamegraph', () => {
    const highlightQuery = 'main';
    const focusedNode: focusedNodeType = Maybe.nothing();

    const flame = new Flamegraph(
      TestData.DiffTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom,
      DefaultPalette
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a zoomed flamegraph', () => {
    const zoom = Maybe.just({ i: 2, j: 8 });
    const focusedNode: focusedNodeType = Maybe.nothing();

    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom,
      DefaultPalette
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a zoomed with fitMode="TAIL"', () => {
    // we need a smaller canvas
    // so that the function names don't fit
    const canvas = createCanvas(300, 0) as unknown as HTMLCanvasElement;
    const fitMode = 'TAIL';
    const focusedNode: focusedNodeType = Maybe.nothing();

    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom,
      DefaultPalette
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  describe('focused', () => {
    it('renders a focused node in the beginning', () => {
      const zoom: zoomType = Maybe.nothing();
      const focusedNode = Maybe.just({ i: 2, j: 0 });

      const flame = new Flamegraph(
        TestData.SimpleTree,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom,
        DefaultPalette
      );

      flame.render();
      expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
    });

    it('renders a focused node (when node is not in the beginning)', () => {
      const zoom: zoomType = Maybe.nothing();
      const focusedNode = Maybe.just({ i: 2, j: 8 });

      const flame = new Flamegraph(
        TestData.SimpleTree,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom,
        DefaultPalette
      );

      flame.render();
      expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
    });

    it('also zooms', () => {
      const focusedNode = Maybe.just({ i: 1, j: 0 });
      const zoom = Maybe.just({ i: 2, j: 0 }); // main.fastFunction

      const flame = new Flamegraph(
        TestData.SimpleTree,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom,
        DefaultPalette
      );

      flame.render();
      expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
    });
  });
});

function canvasToBuffer(canvas: HTMLCanvasElement) {
  const converter = new CanvasConverter(canvas, {
    image: { types: ['png'] },
  });

  return converter.toBuffer();
}
