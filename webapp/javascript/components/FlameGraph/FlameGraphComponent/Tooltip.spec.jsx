/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { diffColorRed, diffColorGreen } from './color';

import Tooltip from './Tooltip';

function TestCanvas(props) {
  const { format } = props;
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
    it("works with a function that hasn't changed", () => {
      const xyToData = () => ({
        title: 'my_function',
        numBarTicks: 10,
        percent: 1,
        left: 100,
        right: 100,
        leftPercent: 10,
        rightPercent: 10,
      });

      render(
        <TestCanvas
          format="double"
          units="samples"
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'my_function'
      );

      expect(screen.getByTestId('flamegraph-tooltip-left')).toHaveTextContent(
        'Left: 100 samples, 1.00 seconds (10%)'
      );
      expect(screen.getByTestId('flamegraph-tooltip-right')).toHaveTextContent(
        'Right: 100 samples, 1.00 seconds (10%)'
      );
    });

    it('works with a function that has been added', () => {
      const xyToData = () => ({
        title: 'my_function',
        numBarTicks: 10,
        percent: 1,
        left: 0,
        right: 100,
        leftPercent: 0,
        rightPercent: 10,
      });

      render(
        <TestCanvas
          format="double"
          units="samples"
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'my_function (new)'
      );

      expect(screen.getByTestId('flamegraph-tooltip-title-diff')).toHaveStyle({
        color: diffColorRed,
      });

      expect(screen.getByTestId('flamegraph-tooltip-left')).toHaveTextContent(
        'Left: 0 samples, < 0.01 seconds (0%)'
      );
      expect(screen.getByTestId('flamegraph-tooltip-right')).toHaveTextContent(
        'Right: 100 samples, 1.00 seconds (10%)'
      );
    });

    it('works with a function that has been removed', () => {
      const xyToData = () => ({
        title: 'my_function',
        numBarTicks: 10,
        percent: 1,
        left: 100,
        right: 0,
        leftPercent: 10,
        rightPercent: 0,
      });

      render(
        <TestCanvas
          format="double"
          units="samples"
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'my_function (removed)'
      );

      expect(screen.getByTestId('flamegraph-tooltip-title-diff')).toHaveStyle({
        color: diffColorGreen,
      });

      expect(screen.getByTestId('flamegraph-tooltip-left')).toHaveTextContent(
        'Left: 100 samples, 1.00 seconds (10%)'
      );
      expect(screen.getByTestId('flamegraph-tooltip-right')).toHaveTextContent(
        'Right: 0 samples, < 0.01 seconds (0%)'
      );
    });

    it('works with a function that became slower', () => {
      const xyToData = () => ({
        title: 'my_function',
        numBarTicks: 10,
        percent: 1,
        left: 100,
        right: 100,
        leftPercent: 10,
        rightPercent: 20,
      });

      render(
        <TestCanvas
          format="double"
          units="samples"
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'my_function (+100.00%)'
      );

      expect(screen.getByTestId('flamegraph-tooltip-title-diff')).toHaveStyle({
        color: diffColorRed,
      });

      expect(screen.getByTestId('flamegraph-tooltip-left')).toHaveTextContent(
        'Left: 100 samples, 1.00 seconds (10%)'
      );
      expect(screen.getByTestId('flamegraph-tooltip-right')).toHaveTextContent(
        'Right: 100 samples, 1.00 seconds (20%)'
      );
    });

    it('works with a function that became faster', () => {
      const xyToData = () => ({
        title: 'my_function',
        numBarTicks: 10,
        percent: 1,
        left: 100,
        right: 100,
        leftPercent: 20,
        rightPercent: 10,
      });

      render(
        <TestCanvas
          format="double"
          units="samples"
          sampleRate={100}
          isWithinBounds={isWithinBounds}
          xyToData={xyToData}
        />
      );

      // since we are mocking the result
      // we don't care exactly where it's hovered
      userEvent.hover(screen.getByTestId('canvas'));

      expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
        'my_function (-50.00%)'
      );

      expect(screen.getByTestId('flamegraph-tooltip-title-diff')).toHaveStyle({
        color: diffColorGreen,
      });

      expect(screen.getByTestId('flamegraph-tooltip-left')).toHaveTextContent(
        'Left: 100 samples, 1.00 seconds (20%)'
      );
      expect(screen.getByTestId('flamegraph-tooltip-right')).toHaveTextContent(
        'Right: 100 samples, 1.00 seconds (10%)'
      );
    });
  });
});
