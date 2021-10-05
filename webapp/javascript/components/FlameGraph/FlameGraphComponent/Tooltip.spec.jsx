/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { diffColorRed, diffColorGreen } from './color';

import Tooltip from './Tooltip';

function TestCanvas(props) {
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

  describe('"single" mode', () => {
    it('renders correctly', () => {
      const xyToData = () => ({
        title: 'function_title',
        numBarTicks: 10,
        percent: 1,
      });

      render(
        <TestCanvas
          format="single"
          units="samples"
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />,
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'function_title',
      );
      expect(screen.getByTestId('flamegraph-tooltip-body')).toHaveTextContent(
        '1, 10 samples, 0.10 seconds',
      );
    });
  });

  describe('"double" mode', () => {
    function renderComponent(d) {
      const xyToData = () => ({
        title: 'my_function',
        numBarTicks: 10,
        percent: 1,
        ...d,
      });

      render(
        <TestCanvas
          format="double"
          units="samples"
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />,
      );
    }

    function assertTooltipContent({ title, diffColor, left, right }) {
      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        title,
      );

      if (diffColor) {
        expect(screen.getByTestId('flamegraph-tooltip-title-diff')).toHaveStyle(
          {
            color: diffColor,
          },
        );
      }

      expect(screen.getByTestId('flamegraph-tooltip-left')).toHaveTextContent(
        left,
      );
      expect(screen.getByTestId('flamegraph-tooltip-right')).toHaveTextContent(
        right,
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
        left: 'Left: 100 samples, 1.00 seconds (10%)',
        right: 'Right: 100 samples, 1.00 seconds (10%)',
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
        right: 'Right: 100 samples, 1.00 seconds (10%)',
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
        left: 'Left: 100 samples, 1.00 seconds (10%)',
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
        left: 'Left: 100 samples, 1.00 seconds (10%)',
        right: 'Right: 100 samples, 1.00 seconds (20%)',
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
        left: 'Left: 100 samples, 1.00 seconds (20%)',
        right: 'Right: 100 samples, 1.00 seconds (10%)',
      });
    });
  });
});
