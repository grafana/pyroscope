import React from 'react';
import { render, screen } from '@testing-library/react';
import NoData from '.';

describe('NoData', () => {
  it('should render correctly', () => {
    render(<NoData />);

    expect(screen.getByTestId('no-data')).toBeInTheDocument();
    expect(screen.getByTestId('no-data')).toHaveTextContent(
      'No data available'
    );
  });
});
