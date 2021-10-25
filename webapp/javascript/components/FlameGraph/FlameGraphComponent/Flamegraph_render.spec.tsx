import CanvasConverter from 'canvas-to-buffer';
import { createCanvas } from 'canvas';
import TestData from './testData';
import Flamegraph from './Flamegraph';

// All tests here refer strictly to the rendering bit of "Flamegraph"

describe("render group:snapshot'", () => {
  // TODO i'm thinking here if we can simply reuse this?
  const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;
  const focusedNode = { i: -1, j: -1 };
  const fitMode = 'HEAD';
  const highlightQuery = '';
  const zoom = { i: -1, j: -1 };

  it('renders a simple flamegraph', () => {
    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom
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
      zoom
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
      zoom
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a highlighted flamegraph', () => {
    const highlightQuery = 'main';
    const focusedNode = { i: -1, j: -1 };

    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a zoomed flamegraph', () => {
    const zoom = { i: 2, j: 8 };
    const focusedNode = { i: -1, j: -1 };

    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('renders a zoomed with fitMode="TAIL"', () => {
    // we need a smaller canvas
    // so that the function names don't fit
    const canvas = createCanvas(300, 0) as unknown as HTMLCanvasElement;
    const fitMode = 'TAIL';
    const focusedNode = { i: -1, j: -1 };

    const flame = new Flamegraph(
      TestData.SimpleTree,
      canvas,
      focusedNode,
      fitMode,
      highlightQuery,
      zoom
    );

    flame.render();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  describe.only('focused', () => {
    it('renders a focused node in the beginning', () => {
      const zoom = { i: -1, j: -1 };

      const focusedNode = { i: 2, j: 0 };

      const flame = new Flamegraph(
        TestData.SimpleTree,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom
      );

      flame.render();
      expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
    });

    it('renders a focused node (when node is not in the beginning)', () => {
      const zoom = { i: -1, j: -1 };

      const focusedNode = { i: 2, j: 8 };

      const flame = new Flamegraph(
        TestData.SimpleTree,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom
      );

      flame.render();
      expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
    });

    it.only('also zooms', () => {
      const focusedNode = { i: 1, j: 0 };
      const zoom = { i: 2, j: 0 }; // main.fastFunction

      const flame = new Flamegraph(
        TestData.SimpleTree,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom
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
