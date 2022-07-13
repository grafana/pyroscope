/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Maybe } from 'true-myth';
import type { Units } from '@pyroscope/models';

import { diffColorRed, diffColorGreen } from './color';
import Tooltip, { TooltipProps } from './Tooltip';
import { DefaultPalette } from './colorPalette';

function TestCanvas(props: Omit<TooltipProps, 'canvasRef'>) {
  const canvasRef = React.useRef<HTMLCanvasElement>(null);

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      <Tooltip
        data-testid="tooltip"
        {...(props as TooltipProps)}
        canvasRef={canvasRef}
      />
    </>
  );
}

describe('Tooltip', () => {
  function executeTooltipTest(
    props: Omit<TooltipProps, 'canvasRef' | 'palette'>,
    expectedData: {
      diff?: { text: string; color: string };
      title: string;
      percent: string | number;
      samples: string;
      formattedValue: string;
    }
  ) {
    render(<TestCanvas {...props} palette={DefaultPalette} />);

    // since we are mocking the result
    // we don't care exactly where it's hovered
    userEvent.hover(screen.getByTestId('canvas'));

    expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
      expectedData.title
    );
    expect(
      screen.getByTestId('flamegraph-tooltip-function-name')
    ).toHaveTextContent(expectedData.title);

    const tableComponent = screen.getByTestId('flamegraph-tooltip-table');
    expect(tableComponent).toContainHTML('table');

    if (expectedData?.diff) {
      expect(tableComponent).toContainHTML('thead');
      expect(tableComponent).toHaveTextContent('BaselineComparisonDiff');

      const diffComponent = screen.getByTestId('flamegraph-tooltip-diff');
      expect(diffComponent).toHaveStyle({ color: expectedData.diff.color });
      expect(diffComponent).toHaveTextContent(expectedData.diff.text);
    }

    const tableHeader = expectedData?.diff ? 'BaselineComparisonDiff' : '';
    const diff = expectedData?.diff ? expectedData.diff.text : '';

    expect(tableComponent).toHaveTextContent(
      tableHeader +
        expectedData.percent +
        diff +
        expectedData.formattedValue +
        expectedData.samples
    );
  }

  describe('"single" mode', () => {
    it('renders correctly', () => {
      const xyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'single' as const,
          name: 'function_title',
          total: 10,
        });

      const tooltipProps = {
        numTicks: 100,
        sampleRate: 100,
        xyToData,
        leftTicks: 100,
        rightTicks: 100,
        format: 'single' as const,
        units: 'samples' as Units,
      };

      const expectedTableData = {
        percent: 'Share of CPU:10%',
        samples: 'Samples:10',
        formattedValue: 'CPU Time:0.10 seconds',
        title: 'function_title',
      };

      executeTooltipTest(tooltipProps, expectedTableData);
    });
  });

  describe('"double" mode', () => {
    it("works with a function that hasn't changed", () => {
      const myxyToData = (x: number, y: number) =>
        Maybe.of({
          format: 'double' as const,
          name: 'my_function',
          totalLeft: 100,
          totalRight: 100,
          barTotal: 100,
        });

      const tooltipProps = {
        numTicks: 100,
        sampleRate: 100,
        xyToData: myxyToData,
        leftTicks: 100010,
        rightTicks: 100,
        format: 'double' as const,
        units: 'samples' as Units,
      };

      const expectedTableData = {
        percent: 'Share of CPU:0.1%100%',
        formattedValue: 'CPU Time:1.00 second1.00 second',
        samples: 'Samples:100100',
        title: 'my_function',
        diff: {
          text: '(+99900.00%)',
          color: 'rgb(200, 0, 0)',
        },
      };

      executeTooltipTest(tooltipProps, expectedTableData);
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

      const tooltipProps = {
        numTicks: 100,
        sampleRate: 100,
        xyToData: myxyToData,
        leftTicks: 1000,
        rightTicks: 1000,
        format: 'double' as const,
        units: 'samples' as Units,
      };

      const expectedTableData = {
        percent: 'Share of CPU:0%10%',
        formattedValue: 'CPU Time:< 0.01 seconds1.00 second',
        samples: 'Samples:0100',
        title: 'my_function',
        diff: {
          text: '(new)',
          color: 'rgb(200, 0, 0)',
        },
      };

      executeTooltipTest(tooltipProps, expectedTableData);
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

      const tooltipProps = {
        numTicks: 100,
        sampleRate: 100,
        xyToData: myxyToData,
        leftTicks: 1000,
        rightTicks: 1000,
        format: 'double' as const,
        units: 'samples' as Units,
      };

      const expectedTableData = {
        percent: 'Share of CPU:10%0%',
        formattedValue: 'CPU Time:1.00 second< 0.01 seconds',
        samples: 'Samples:1000',
        title: 'my_function',
        diff: {
          text: '(removed)',
          color: 'rgb(0, 170, 0)',
        },
      };

      executeTooltipTest(tooltipProps, expectedTableData);
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

      const tooltipProps = {
        numTicks: 100,
        sampleRate: 100,
        xyToData: myxyToData,
        leftTicks: 1000,
        rightTicks: 1000,
        format: 'double' as const,
        units: 'samples' as Units,
      };

      const expectedTableData = {
        percent: 'Share of CPU:10%20%',
        formattedValue: 'CPU Time:1.00 second2.00 seconds',
        samples: 'Samples:100200',
        title: 'my_function',
        diff: {
          text: '(+100.00%)',
          color: 'rgb(200, 0, 0)',
        },
      };

      executeTooltipTest(tooltipProps, expectedTableData);
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

      const tooltipProps = {
        numTicks: 100,
        sampleRate: 100,
        xyToData: myxyToData,
        leftTicks: 1000,
        rightTicks: 1000,
        format: 'double' as const,
        units: 'samples' as Units,
      };

      const expectedTableData = {
        percent: 'Share of CPU:20%10%',
        formattedValue: 'CPU Time:2.00 seconds1.00 second',
        samples: 'Samples:200100',
        title: 'my_function',
        diff: {
          text: '(-50.00%)',
          color: 'rgb(0, 170, 0)',
        },
      };

      executeTooltipTest(tooltipProps, expectedTableData);
    });
  });
});
