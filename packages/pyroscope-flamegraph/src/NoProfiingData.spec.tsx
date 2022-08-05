import React from 'react';
import { render, screen } from '@testing-library/react';
import NoProfilingData from './NoProfilingData';

describe('NoProfilingData', () => {
  it('should render correctly', () => {
    render(<NoProfilingData />);

    expect(screen.getByTestId('no-profiling-data')).toBeInTheDocument();
    expect(screen.getByTestId('no-profiling-data')).toHaveTextContent(
      'No profiling data available for this application / time range.'
    );
  });
});
