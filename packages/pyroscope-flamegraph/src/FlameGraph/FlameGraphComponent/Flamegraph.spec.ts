import { Maybe } from 'true-myth';
import Flamegraph from './Flamegraph';
import { BAR_HEIGHT } from './constants';
import { DefaultPalette } from './colorPalette';
import TestData from './testData';

jest.mock('./Flamegraph_render');
const throwUnwrapErr = () => {
  throw new Error('Failed to unwrap');
};

type focusedNodeType = ConstructorParameters<typeof Flamegraph>[2];
type zoomType = ConstructorParameters<typeof Flamegraph>[5];

describe('Flamegraph', () => {
  let canvas: any;
  let flame: Flamegraph;
  const CANVAS_WIDTH = 600;
  const CANVAS_HEIGHT = 300;

  describe('isWithinBounds', () => {
    beforeEach(() => {
      canvas = document.createElement('canvas');
      canvas.width = CANVAS_WIDTH;
      canvas.height = CANVAS_HEIGHT;

      const fitMode = 'HEAD';
      const highlightQuery = '';
      const focusedNode: focusedNodeType = Maybe.nothing();
      const zoom = Maybe.of({ i: 2, j: 8 });

      flame = new Flamegraph(
        TestData.ComplexTree,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom,
        DefaultPalette
      );

      flame.render();
    });

    it('handles within canvas', () => {
      expect(flame.isWithinBounds(0, 0)).toBe(true);
      expect(flame.isWithinBounds(CANVAS_WIDTH - 1, 0)).toBe(true);
      expect(flame.isWithinBounds(-1, 0)).toBe(false);
      expect(flame.isWithinBounds(0, -1)).toBe(false);
      expect(flame.isWithinBounds(-1, -1)).toBe(false);
    });

    it('handles within canvas but outside the flamegraph', () => {
      // this test is a bit difficult to visually
      // you just have to know that it has the format such as
      //
      // | | (level 3)
      // |_| (level 4)
      //     (level 5)
      expect(flame.isWithinBounds(0, BAR_HEIGHT * 3 + 1)).toBe(true);
      expect(flame.isWithinBounds(0, BAR_HEIGHT * 4 + 1)).toBe(true);
      expect(flame.isWithinBounds(0, BAR_HEIGHT * 5 + 1)).toBe(false);
    });
  });

  describe('xyToBarData', () => {
    describe('normal', () => {
      beforeAll(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        const fitMode = 'HEAD';
        const highlightQuery = '';
        const zoom: zoomType = Maybe.nothing();
        const focusedNode: focusedNodeType = Maybe.nothing();

        flame = new Flamegraph(
          TestData.SimpleTree,
          canvas,
          focusedNode,
          fitMode,
          highlightQuery,
          zoom,
          DefaultPalette
        );

        flame.render();
      });

      it('works with the first bar (total)', () => {
        const got = flame.xyToBar(0, 0).unwrapOrElse(throwUnwrapErr);

        expect(got.x).toBe(0);
        expect(got.y).toBe(0);
        expect(got.width).toBeCloseTo(CANVAS_WIDTH);
      });

      it('works a full bar (runtime.main)', () => {
        // 2nd line,
        const got = flame
          .xyToBar(0, BAR_HEIGHT + 1)
          .unwrapOrElse(throwUnwrapErr);

        expect(got.x).toBe(0);
        expect(got.y).toBe(22);
        expect(got.width).toBeCloseTo(CANVAS_WIDTH);
      });

      it('works with (main.fastFunction)', () => {
        // 3nd line, 'slowFunction'
        const got = flame
          .xyToBar(1, BAR_HEIGHT * 2 + 1)
          .unwrapOrElse(throwUnwrapErr);

        expect(got.x).toBe(0);
        expect(got.y).toBe(44);
        expect(got.width).toBeCloseTo(129.95951417004048);
      });

      it('works with (main.slowFunction)', () => {
        // 3nd line, 'slowFunction'
        const got = flame
          .xyToBar(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1)
          .unwrapOrElse(throwUnwrapErr);

        expect(got.x).toBeCloseTo(131.78);
        expect(got.y).toBe(44);
        expect(got.width).toBeCloseTo(468.218);
      });

      describe('boundary testing', () => {
        const cases = [
          [0, 0],
          [CANVAS_WIDTH, 0],
          [1, BAR_HEIGHT],
          [CANVAS_WIDTH, BAR_HEIGHT],
          [CANVAS_WIDTH / 2, BAR_HEIGHT / 2],
        ];
        test.each(cases)(
          'given %p and %p as arguments, returns the total bar',
          (i: number, j: number) => {
            const got = flame.xyToBar(i, j).unwrapOrElse(throwUnwrapErr);
            expect(got).toMatchObject({
              i: 0,
              j: 0,
              x: 0,
              y: 0,
            });

            expect(got.width).toBeCloseTo(CANVAS_WIDTH);
          }
        );
      });
    });

    describe('focused', () => {
      describe('on the first row (runtime.main)', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom: zoomType = Maybe.nothing();
          const focusedNode = Maybe.just({ i: 1, j: 0 });

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom,
            DefaultPalette
          );

          flame.render();
        });

        it('works with the first bar (total)', () => {
          const got = flame.xyToBar(0, 0).unwrapOrElse(throwUnwrapErr);

          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with a full bar (runtime.main)', () => {
          // 2nd line,
          const got = flame
            .xyToBar(0, BAR_HEIGHT + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 1,
            j: 0,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        //
        it('works with (main.fastFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame
            .xyToBar(1, BAR_HEIGHT * 2 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 2,
            j: 0,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(129.95951417004048);
        });
        //
        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame
            .xyToBar(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
          });
          expect(got.x).toBeCloseTo(131.78);
          expect(got.y).toBe(44);
          expect(got.width).toBeCloseTo(468.218);
        });
      });

      describe('on main.slowFunction', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom: zoomType = Maybe.nothing();
          const focusedNode = Maybe.just({ i: 2, j: 8 });

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom,
            DefaultPalette
          );

          flame.render();
        });

        it('works with the first row (total)', () => {
          const got = flame.xyToBar(0, 0).unwrapOrElse(throwUnwrapErr);
          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with itself as second row (main.slowFunction)', () => {
          // 2nd line,
          const got = flame
            .xyToBar(1, BAR_HEIGHT + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with its child as third row (main.work)', () => {
          // 2nd line,
          const got = flame
            .xyToBar(1, BAR_HEIGHT * 2 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 3,
            j: 8,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
      });
    });

    describe('zoomed', () => {
      describe('on the first row (runtime.main)', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';

          const zoom: zoomType = Maybe.of({ i: 1, j: 0 });
          const focusedNode: focusedNodeType = Maybe.nothing();

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom,
            DefaultPalette
          );

          flame.render();
        });

        it('works with the first bar (total)', () => {
          const got = flame.xyToBar(0, 0).unwrapOrElse(throwUnwrapErr);
          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        it('works with a full bar (runtime.main)', () => {
          // 2nd line,
          const got = flame
            .xyToBar(0, BAR_HEIGHT + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 1,
            j: 0,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        //
        it('works with (main.fastFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame
            .xyToBar(1, BAR_HEIGHT * 2 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 2,
            j: 0,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(129.95951417004048);
        });
        //
        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame
            .xyToBar(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
          });
          expect(got.x).toBeCloseTo(131.78);
          expect(got.y).toBe(44);
          expect(got.width).toBeCloseTo(468.218);
        });
      });

      describe('on main.slowFunction', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom = Maybe.of({ i: 2, j: 8 });
          const focusedNode: focusedNodeType = Maybe.nothing();

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom,
            DefaultPalette
          );

          flame.render();
        });

        it('works with the first bar (total)', () => {
          const got = flame.xyToBar(0, 0).unwrapOrElse(throwUnwrapErr);
          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        it('works with a full bar (runtime.main)', () => {
          // 2nd line,
          const got = flame
            .xyToBar(0, BAR_HEIGHT + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 1,
            j: 0,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        //
        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame
            .xyToBar(1, BAR_HEIGHT * 2 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with main.work (child of main.slowFunction)', () => {
          // 4th line, 'main.work'
          // TODO why 2??
          const got = flame
            .xyToBar(1, BAR_HEIGHT * 3 + 2)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 3,
            j: 8,
            x: 0,
            y: 66,
          });
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
      });
    });

    describe('focused+zoomed', () => {
      describe('focused on the first row (runtime.main), zoomed on the third row (main.slowFunction)', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom = Maybe.of({ i: 2, j: 8 });
          const focusedNode = Maybe.of({ i: 1, j: 0 });

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom,
            DefaultPalette
          );

          flame.render();
        });

        it('works with the first bar (total)', () => {
          const got = flame.xyToBar(0, 0).unwrapOrElse(throwUnwrapErr);
          expect(got).toMatchObject({
            x: 0,
            y: 0,
            i: 0,
            j: 0,
          });
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with a full bar (runtime.main)', () => {
          // 2nd line,
          const got = flame
            .xyToBar(0, BAR_HEIGHT + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 1,
            j: 0,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame
            .xyToBar(1, BAR_HEIGHT * 2 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame
            .xyToBar(1, BAR_HEIGHT * 3 + 1)
            .unwrapOrElse(throwUnwrapErr);

          expect(got).toMatchObject({
            i: 3,
            j: 8,
            x: 0,
            y: 66,
          });
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
      });
    });
  });
});
