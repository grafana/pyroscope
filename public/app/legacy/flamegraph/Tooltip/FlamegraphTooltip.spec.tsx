/* eslint-disable react/jsx-props-no-spreading */
import React, { useRef } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Maybe } from 'true-myth';
import type { Units } from '@pyroscope/legacy/models';

import FlamegraphTooltip, { FlamegraphTooltipProps } from './FlamegraphTooltip';
import { DefaultPalette } from '../';

function TestCanvas(props: Omit<FlamegraphTooltipProps, 'canvasRef'>) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      <FlamegraphTooltip
        {...(props as FlamegraphTooltipProps)}
        canvasRef={canvasRef}
      />
    </>
  );
}

describe('FlamegraphTooltip', () => {
  const renderCanvas = (
    props: Omit<FlamegraphTooltipProps, 'canvasRef' | 'palette'>
  ) => render(<TestCanvas {...props} palette={DefaultPalette} />);

  it('should render FlamegraphTooltip with single format', () => {
    const xyToData = (x: number, y: number) =>
      Maybe.of({
        format: 'single' as const,
        name: 'function_title',
        total: 10,
      });

    const props = {
      numTicks: 100,
      sampleRate: 100,
      xyToData,
      leftTicks: 100,
      rightTicks: 100,
      format: 'single' as const,
      units: 'samples' as Units,
    };

    renderCanvas(props);

    userEvent.hover(screen.getByTestId('canvas'));

    expect(screen.getByTestId('tooltip')).toBeInTheDocument();
  });

  it('should render FlamegraphTooltip with double format', () => {
    const xyToData = (x: number, y: number) =>
      Maybe.of({
        format: 'double' as const,
        name: 'my_function',
        totalLeft: 100,
        totalRight: 0,
        barTotal: 100,
      });

    const props = {
      numTicks: 100,
      sampleRate: 100,
      xyToData,
      leftTicks: 1000,
      rightTicks: 1000,
      format: 'double' as const,
      units: 'samples' as Units,
    };

    renderCanvas(props);

    userEvent.hover(screen.getByTestId('canvas'));

    expect(screen.getByTestId('tooltip')).toBeInTheDocument();
  });
});
