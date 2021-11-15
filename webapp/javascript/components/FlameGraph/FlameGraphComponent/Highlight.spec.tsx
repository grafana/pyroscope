/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Option } from 'prelude-ts';

import Highlight, { HighlightProps } from './Highlight';

function TestComponent(props: Omit<HighlightProps, 'canvasRef'>) {
  const canvasRef = React.useRef();

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      <Highlight canvasRef={canvasRef} {...props} />
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
        zoom={Option.none()}
      />
    );

    // hover over a bar
    xyToHighlightData.mockReturnValueOnce(
      Option.of({
        left: 10,
        top: 5,
        width: 100,
      })
    );
    userEvent.hover(screen.getByTestId('canvas'));
    expect(screen.getByTestId('flamegraph-highlight')).toBeVisible();
    expect(screen.getByTestId('flamegraph-highlight')).toHaveStyle({
      height: '50px',
      left: '10px',
      top: '5px',
      width: '100px',
    });

    // hover outside the canvas
    xyToHighlightData.mockReturnValueOnce(Option.none());
    userEvent.hover(screen.getByTestId('canvas'));
    expect(screen.getByTestId('flamegraph-highlight')).not.toBeVisible();
  });
});
