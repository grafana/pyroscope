import CanvasConverter from 'canvas-to-buffer';
import { createCanvas } from 'canvas';
import { Option } from 'prelude-ts';
import TestData from './testData';
import Flamegraph from './Flamegraph';

type focusedNodeType = ConstructorParameters<typeof Flamegraph>[2];
type zoomType = ConstructorParameters<typeof Flamegraph>[5];

// All tests here refer strictly to the rendering bit of "Flamegraph"
describe("render group:snapshot'", () => {
  // TODO i'm thinking here if we can simply reuse this?
  const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;
  const fitMode = 'HEAD';
  const highlightQuery = '';
  const zoom: zoomType = Option.none();
  const focusedNode: focusedNodeType = Option.none();

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
    const focusedNode: focusedNodeType = Option.none();

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

  it('renders a highlighted double flamegraph', () => {
    const highlightQuery = 'main';
    const focusedNode: focusedNodeType = Option.none();

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

  it('renders a zoomed flamegraph', () => {
    const zoom = Option.some({ i: 2, j: 8 });
    const focusedNode: focusedNodeType = Option.none();

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
    const focusedNode: focusedNodeType = Option.none();

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

  describe('focused', () => {
    it('renders a focused node in the beginning', () => {
      const zoom: zoomType = Option.none();
      const focusedNode = Option.some({ i: 2, j: 0 });

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
      const zoom: zoomType = Option.none();
      const focusedNode = Option.some({ i: 2, j: 8 });

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

    it('also zooms', () => {
      const focusedNode = Option.some({ i: 1, j: 0 });
      const zoom = Option.some({ i: 2, j: 0 }); // main.fastFunction

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
