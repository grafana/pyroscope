/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Maybe } from 'true-myth';

import Highlight, { HighlightProps } from './Highlight';

function TestComponent(props: Omit<HighlightProps, 'canvasRef'>) {
  const canvasRef = React.useRef<HTMLCanvasElement>(null);

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      {canvasRef && <Highlight canvasRef={canvasRef} {...props} />}
    </>
  );
}

describe('Highlight', () => {
  it('works', () => {
    const xyToHighlightData = jest.fn();
    render(
      <TestComponent
        barHeight={50}
        xyToHighlightData={xyToHighlightData}
        zoom={Maybe.nothing()}
      />
    );

    // hover over a bar
    xyToHighlightData.mockReturnValueOnce(
      Maybe.of({
        left: 10,
        top: 5,
        width: 100,
      })
    );
    act(() => userEvent.hover(screen.getByTestId('canvas')));
    expect(screen.getByTestId('flamegraph-highlight')).toBeVisible();
    expect(screen.getByTestId('flamegraph-highlight')).toHaveStyle({
      height: '50px',
      left: '10px',
      top: '5px',
      width: '100px',
    });

    // hover outside the canvas
    xyToHighlightData.mockReturnValueOnce(Maybe.nothing());
    act(() => userEvent.hover(screen.getByTestId('canvas')));
    expect(screen.getByTestId('flamegraph-highlight')).not.toBeVisible();
  });
});
