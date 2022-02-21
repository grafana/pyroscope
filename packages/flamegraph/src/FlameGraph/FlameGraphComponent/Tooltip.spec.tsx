/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Maybe } from '@utils/fp';

import { diffColorRed, diffColorGreen } from './color';
import { Units } from '@utils/format';
import Tooltip, { TooltipProps } from './Tooltip';
import { DefaultPalette } from './colorPalette';

// Omit<TooltipProps, 'canvasRef'>) wasn't working
// so for testing let's pass canvasRef = undefined
function TestCanvas(props: TooltipProps) {
  const canvasRef = React.useRef();

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      <Tooltip data-testid="tooltip" {...props} canvasRef={canvasRef} />
    </>
  );
}

describe('Tooltip', () => {
  describe('"single" mode', () => {
    it('renders correctly', () => {
      const xyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'single' as const,
          name: 'function_title',
          total: 10,
        });

      render(
        <TestCanvas
          canvasRef={undefined}
          format="single"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          xyToData={xyToData}
          leftTicks={100}
          rightTicks={100}
          palette={DefaultPalette}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'function_title'
      );
      expect(screen.getByTestId('flamegraph-tooltip-body')).toHaveTextContent(
        '10%, 10 samples, 0.10 seconds'
      );
    });
  });

  describe('"double" mode', () => {
    function assertTooltipContent({ title, diffColor, left, right }) {
      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        title
      );

      if (diffColor) {
        expect(screen.getByTestId('flamegraph-tooltip-title-diff')).toHaveStyle(
          {
            color: diffColor,
          }
        );
      }

      expect(screen.getByTestId('flamegraph-tooltip-left')).toHaveTextContent(
        left
      );
      expect(screen.getByTestId('flamegraph-tooltip-right')).toHaveTextContent(
        right
      );
    }

    it("works with a function that hasn't changed", () => {
      const myxyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'double' as const,
          name: 'my_function',
          totalLeft: 100,
          totalRight: 100,
          barTotal: 100,
        });

      render(
        <TestCanvas
          canvasRef={undefined}
          format="double"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          leftTicks={1000}
          rightTicks={1000}
          xyToData={myxyToData}
          palette={DefaultPalette}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      assertTooltipContent({
        title: 'my_function',
        diffColor: undefined,
        left: 'Left: 100 samples, 1.00 second (10%)',
        right: 'Right: 100 samples, 1.00 second (10%)',
      });
    });

    it('works with a function that has been added', () => {
      const myxyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'double' as const,
          name: 'my_function',
          totalLeft: 0,
          totalRight: 100,
          barTotal: 100,
        });

      render(
        <TestCanvas
          canvasRef={undefined}
          format="double"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          leftTicks={1000}
          rightTicks={1000}
          xyToData={myxyToData}
          palette={DefaultPalette}
        />
      );
      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      assertTooltipContent({
        title: 'my_function (new)',
        diffColor: diffColorRed,
        left: 'Left: 0 samples, < 0.01 seconds (0%)',
        right: 'Right: 100 samples, 1.00 second (10%)',
      });
    });

    it('works with a function that has been removed', () => {
      const myxyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'double' as const,
          name: 'my_function',
          totalLeft: 100,
          totalRight: 0,
          barTotal: 100,
        });

      render(
        <TestCanvas
          canvasRef={undefined}
          format="double"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          leftTicks={1000}
          rightTicks={1000}
          xyToData={myxyToData}
          palette={DefaultPalette}
        />
      );
      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      assertTooltipContent({
        title: 'my_function (removed)',
        diffColor: diffColorGreen,
        left: 'Left: 100 samples, 1.00 second (10%)',
        right: 'Right: 0 samples, < 0.01 seconds (0%)',
      });
    });

    it('works with a function that became slower', () => {
      const myxyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'double' as const,
          name: 'my_function',
          totalLeft: 100,
          totalRight: 200,
          barTotal: 100,
        });

      render(
        <TestCanvas
          canvasRef={undefined}
          format="double"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          leftTicks={1000}
          rightTicks={1000}
          xyToData={myxyToData}
          palette={DefaultPalette}
        />
      );
      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      assertTooltipContent({
        title: 'my_function (+100.00%)',
        diffColor: diffColorRed,
        left: 'Left: 100 samples, 1.00 second (10%)',
        right: 'Right: 200 samples, 2.00 seconds (20%)',
      });
    });

    it('works with a function that became faster', () => {
      const myxyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'double' as const,
          name: 'my_function',
          totalLeft: 200,
          totalRight: 100,
          barTotal: 100,
        });

      render(
        <TestCanvas
          canvasRef={undefined}
          format="double"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          leftTicks={1000}
          rightTicks={1000}
          xyToData={myxyToData}
          palette={DefaultPalette}
        />
      );
      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      assertTooltipContent({
        title: 'my_function (-50.00%)',
        diffColor: diffColorGreen,
        left: 'Left: 200 samples, 2.00 seconds (20%)',
        right: 'Right: 100 samples, 1.00 second (10%)',
      });
    });
  });
});
