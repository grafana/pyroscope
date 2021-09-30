/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

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
  it('works', () => {
    render(<TestCanvas />);
  });

  it('renders in single mode', () => {
    const isWithinBounds = () => true;
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

    userEvent.click(screen.getByTestId('canvas'));

    expect(screen.getByTestId('flamegraph-tooltip-title')).toHaveTextContent(
      'function_title'
    );
    expect(screen.getByTestId('flamegraph-tooltip-body')).toHaveTextContent(
      '1, 10 samples, 0.10 seconds'
    );
  });
});
