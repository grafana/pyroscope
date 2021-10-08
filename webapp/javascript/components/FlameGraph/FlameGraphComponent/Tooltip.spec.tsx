/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { diffColorRed, diffColorGreen } from './color';
import { Units } from '../../../util/format';

import Tooltip, { TooltipProps } from './Tooltip';

function TestCanvas(props: Omit<TooltipProps, 'canvasRef'>) {
  const canvasRef = React.useRef();

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      <Tooltip data-testid="tooltip" canvasRef={canvasRef} {...props} />
    </>
  );
}

describe('Tooltip', () => {
  const isWithinBounds = () => true;

  // this test handles a case where the app has changed
  // but the unit stayed the same
  it('updates units correctly', () => {
    const xyToData = (format: 'single', x: number, y: number) => ({
      format,
      title: 'function_title',
      numBarTicks: 10,
      percent: 1,
    });

    const { rerender } = render(
      <TestCanvas
        format="single"
        units={Units.Samples}
        sampleRate={100}
        numTicks={100}
        isWithinBounds={isWithinBounds}
        xyToData={xyToData}
      />
    );

    // since we are mocking the result
    // we don't care exactly where it's hovered
    userEvent.hover(screen.getByTestId('canvas'));

    expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
      'function_title'
    );
    expect(screen.getByTestId('flamegraph-tooltip-body')).toHaveTextContent(
      '1, 10 samples, 0.10 seconds'
    );

    rerender(
      <TestCanvas
        format="single"
        units={Units.Objects}
        numTicks={1000}
        sampleRate={100}
        isWithinBounds={isWithinBounds}
        xyToData={xyToData}
      />
    );

    userEvent.hover(screen.getByTestId('canvas'));

    expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
      'function_title'
    );
    expect(screen.getByTestId('flamegraph-tooltip-body')).toHaveTextContent(
      '1, 10 samples, 0.01 K'
    );
  });

  describe('"single" mode', () => {
    it('renders correctly', () => {
      const xyToData = (format: 'single', x: number, y: number) => ({
        format,
        title: 'function_title',
        numBarTicks: 10,
        percent: 1,
      });

      render(
        <TestCanvas
          format="single"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'function_title'
      );
      expect(screen.getByTestId('flamegraph-tooltip-body')).toHaveTextContent(
        '1, 10 samples, 0.10 seconds'
      );
    });
  });

  describe('"double" mode', () => {
    function renderComponent(d) {
      const xyToData = () => ({
        title: 'my_function',
        numBarTicks: 10,
        percent: 1,
        format: 'double',
        ...d,
      });

      render(
        <TestCanvas
          format="double"
          units={Units.Samples}
          numTicks={100}
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />
      );
    }

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
      renderComponent({
        left: 100,
        right: 100,
        leftPercent: 10,
        rightPercent: 10,
      });

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
      renderComponent({
        left: 0,
        right: 100,
        leftPercent: 0,
        rightPercent: 10,
      });

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
      renderComponent({
        left: 100,
        right: 0,
        leftPercent: 10,
        rightPercent: 0,
      });

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
      renderComponent({
        left: 100,
        right: 100,
        leftPercent: 10,
        rightPercent: 20,
      });

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      assertTooltipContent({
        title: 'my_function (+100.00%)',
        diffColor: diffColorRed,
        left: 'Left: 100 samples, 1.00 second (10%)',
        right: 'Right: 100 samples, 1.00 second (20%)',
      });
    });

    it('works with a function that became faster', () => {
      renderComponent({
        left: 100,
        right: 100,
        leftPercent: 20,
        rightPercent: 10,
      });

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      assertTooltipContent({
        title: 'my_function (-50.00%)',
        diffColor: diffColorGreen,
        left: 'Left: 100 samples, 1.00 second (20%)',
        right: 'Right: 100 samples, 1.00 second (10%)',
      });
    });
  });
});
