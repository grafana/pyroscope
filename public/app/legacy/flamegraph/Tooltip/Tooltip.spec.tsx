/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { Tooltip, TooltipProps } from './Tooltip';

function TestCanvas(props: Omit<TooltipProps, 'dataSourceRef'>) {
  const canvasRef = React.useRef<HTMLCanvasElement>(null);

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      <Tooltip
        data-testid="tooltip"
        {...(props as TooltipProps)}
        dataSourceRef={canvasRef}
      />
    </>
  );
}

function TestTable(props: Omit<TooltipProps, 'dataSourceRef'>) {
  const tableBodyRef = React.useRef<HTMLTableSectionElement>(null);

  return (
    <>
      <table>
        <tbody data-testid="table-body" ref={tableBodyRef}>
          <tr>
            <td>text</td>
          </tr>
        </tbody>
      </table>
      <Tooltip
        data-testid="tooltip"
        {...(props as TooltipProps)}
        dataSourceRef={tableBodyRef}
      />
    </>
  );
}

describe('Tooltip', () => {
  describe('flamegraph tooltip', () => {
    it("'single' mode with default settings renders correctly", () => {
      render(
        <TestCanvas
          clickInfoSide="right"
          setTooltipContent={(setContent) => {
            setContent({
              title: {
                text: 'function_title',
                diff: {
                  text: '',
                  color: '',
                },
              },
              tooltipData: [
                {
                  units: 'samples',
                  percent: 100,
                  samples: '100',
                  formattedValue: '1 second',
                  tooltipType: 'flamegraph',
                },
              ],
            });
          }}
        />
      );

      expect(screen.getByTestId('tooltip')).toBeInTheDocument();

      act(() => userEvent.hover(screen.getByTestId('canvas')));

      expect(screen.getByTestId('tooltip-title')).toHaveTextContent(
        'function_title'
      );
      expect(screen.getByTestId('tooltip-function-name')).toHaveTextContent(
        'function_title'
      );
      expect(screen.getByTestId('tooltip-table')).toHaveTextContent(
        'Share of CPU:100CPU Time:1 secondSamples:100'
      );
      expect(screen.getByTestId('tooltip-footer')).toHaveTextContent(
        'Right click for more node viewing options'
      );
    });

    describe("'double' mode with default settings", () => {
      it("works with a function that hasn't changed", () => {
        render(
          <TestCanvas
            clickInfoSide="right"
            setTooltipContent={(setContent) => {
              setContent({
                title: {
                  text: 'function_title',
                  diff: {
                    text: '(+99900.00%)',
                    color: 'rgb(200, 0, 0)',
                  },
                },
                tooltipData: [
                  {
                    percent: '0.1%',
                    samples: '100',
                    units: 'samples',
                    formattedValue: '1.00 second',
                    tooltipType: 'flamegraph',
                  },
                  {
                    percent: '100%',
                    samples: '100',
                    units: 'samples',
                    formattedValue: '1.00 second',
                    tooltipType: 'flamegraph',
                  },
                ],
              });
            }}
          />
        );

        expect(screen.getByTestId('tooltip')).toBeInTheDocument();

        act(() => userEvent.hover(screen.getByTestId('canvas')));

        expect(screen.getByTestId('tooltip-title')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-function-name')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-table')).toHaveTextContent(
          'BaselineComparisonDiffShare of CPU:0.1%100%(+99900.00%)CPU Time:1.00 second1.00 secondSamples:100100'
        );
        expect(screen.getByTestId('tooltip-footer')).toHaveTextContent(
          'Right click for more node viewing options'
        );
      });

      it('works with a function that has been added', () => {
        render(
          <TestCanvas
            clickInfoSide="right"
            setTooltipContent={(setContent) => {
              setContent({
                title: {
                  text: 'function_title',
                  diff: {
                    text: '(new)',
                    color: 'rgb(200, 0, 0)',
                  },
                },
                tooltipData: [
                  {
                    percent: '0%',
                    samples: '0',
                    units: 'samples',
                    formattedValue: '< 0.01 seconds',
                    tooltipType: 'flamegraph',
                  },
                  {
                    percent: '10%',
                    samples: '100',
                    units: 'samples',
                    formattedValue: '1.00 second',
                    tooltipType: 'flamegraph',
                  },
                ],
              });
            }}
          />
        );

        expect(screen.getByTestId('tooltip')).toBeInTheDocument();

        act(() => userEvent.hover(screen.getByTestId('canvas')));
        expect(screen.getByTestId('tooltip-title')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-function-name')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-table')).toHaveTextContent(
          'BaselineComparisonDiffShare of CPU:0%10%(new)CPU Time:< 0.01 seconds1.00 secondSamples:0100'
        );
        expect(screen.getByTestId('tooltip-footer')).toHaveTextContent(
          'Right click for more node viewing options'
        );
      });

      it('works with a function that has been removed', () => {
        render(
          <TestCanvas
            clickInfoSide="right"
            setTooltipContent={(setContent) => {
              setContent({
                title: {
                  text: 'function_title',
                  diff: {
                    text: '(removed)',
                    color: 'rgb(0, 170, 0)',
                  },
                },
                tooltipData: [
                  {
                    percent: '10%',
                    samples: '100',
                    units: 'samples',
                    formattedValue: '1.00 second',
                    tooltipType: 'flamegraph',
                  },
                  {
                    percent: '0%',
                    samples: '0',
                    units: 'samples',
                    formattedValue: '< 0.01 seconds',
                    tooltipType: 'flamegraph',
                  },
                ],
              });
            }}
          />
        );

        expect(screen.getByTestId('tooltip')).toBeInTheDocument();

        act(() => userEvent.hover(screen.getByTestId('canvas')));

        expect(screen.getByTestId('tooltip-title')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-function-name')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-table')).toHaveTextContent(
          'BaselineComparisonDiffShare of CPU:10%0%(removed)CPU Time:1.00 second< 0.01 secondsSamples:1000'
        );
        expect(screen.getByTestId('tooltip-footer')).toHaveTextContent(
          'Right click for more node viewing options'
        );
      });

      it('works with a function that became slower', () => {
        render(
          <TestCanvas
            clickInfoSide="right"
            setTooltipContent={(setContent) => {
              setContent({
                title: {
                  text: 'function_title',
                  diff: {
                    text: '(+100.00%)',
                    color: 'rgb(200, 0, 0)',
                  },
                },
                tooltipData: [
                  {
                    percent: '10%',
                    samples: '100',
                    units: 'samples',
                    formattedValue: '1.00 second',
                    tooltipType: 'flamegraph',
                  },
                  {
                    percent: '20%',
                    samples: '200',
                    units: 'samples',
                    formattedValue: '2.00 seconds',
                    tooltipType: 'flamegraph',
                  },
                ],
              });
            }}
          />
        );

        expect(screen.getByTestId('tooltip')).toBeInTheDocument();

        act(() => userEvent.hover(screen.getByTestId('canvas')));

        expect(screen.getByTestId('tooltip-title')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-function-name')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-table')).toHaveTextContent(
          'BaselineComparisonDiffShare of CPU:10%20%(+100.00%)CPU Time:1.00 second2.00 secondsSamples:100200'
        );
        expect(screen.getByTestId('tooltip-footer')).toHaveTextContent(
          'Right click for more node viewing options'
        );
      });

      it('works with a function that became faster', () => {
        render(
          <TestCanvas
            clickInfoSide="right"
            setTooltipContent={(setContent) => {
              setContent({
                title: {
                  text: 'function_title',
                  diff: {
                    text: '(-50.00%)',
                    color: 'rgb(0, 170, 0)',
                  },
                },
                tooltipData: [
                  {
                    percent: '20%',
                    samples: '200',
                    units: 'samples',
                    formattedValue: '2.00 second',
                    tooltipType: 'flamegraph',
                  },
                  {
                    percent: '10%',
                    samples: '100',
                    units: 'samples',
                    formattedValue: '1.00 seconds',
                    tooltipType: 'flamegraph',
                  },
                ],
              });
            }}
          />
        );

        expect(screen.getByTestId('tooltip')).toBeInTheDocument();

        act(() => userEvent.hover(screen.getByTestId('canvas')));

        expect(screen.getByTestId('tooltip-title')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-function-name')).toHaveTextContent(
          'function_title'
        );
        expect(screen.getByTestId('tooltip-table')).toHaveTextContent(
          'BaselineComparisonDiffShare of CPU:20%10%(-50.00%)CPU Time:2.00 second1.00 secondsSamples:200100'
        );
        expect(screen.getByTestId('tooltip-footer')).toHaveTextContent(
          'Right click for more node viewing options'
        );
      });
    });
  });

  describe('table tooltip', () => {
    it("'single' mode with custom settings renders correctly", () => {
      render(
        <TestTable
          clickInfoSide="left"
          shouldShowFooter={false}
          shouldShowTitle={false}
          setTooltipContent={(setContent) => {
            setContent({
              title: {
                text: 'function_title',
                diff: {
                  text: '',
                  color: '',
                },
              },
              tooltipData: [
                {
                  total: '2 seconds (100%)',
                  self: '1 second (50%)',
                  tooltipType: 'table',
                  units: 'samples',
                },
              ],
            });
          }}
        />
      );

      expect(screen.getByTestId('tooltip')).toBeInTheDocument();

      act(() => userEvent.hover(screen.getByTestId('table-body')));

      expect(screen.getByTestId('tooltip-function-name')).toHaveTextContent(
        'function_title'
      );
      expect(screen.getByTestId('tooltip-table')).toHaveTextContent(
        'Self (% of total CPU)Total (% of total CPU)CPU Time:1 second (50%)2 seconds (100%)'
      );
    });
  });
});
