import '@testing-library/jest-dom';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React, { useState } from 'react';

import FlameGraphHeader from './FlameGraphHeader';

describe('FlameGraphHeader', () => {
  const FlameGraphHeaderWithProps = () => {
    const [query, setQuery] = useState('');

    return (
      <FlameGraphHeader
        query={query}
        setQuery={setQuery}
        setTopLevelIndex={jest.fn()}
        setRangeMin={jest.fn()}
        setRangeMax={jest.fn()}
      />
    );
  };

  it('reset button should remove query text', async () => {
    render(<FlameGraphHeaderWithProps />);
    await userEvent.type(screen.getByPlaceholderText('Search..'), 'abc');
    expect(screen.getByDisplayValue('abc')).toBeInTheDocument();
    screen.getByRole('button', { name: /Reset/i }).click();
    expect(screen.queryByDisplayValue('abc')).not.toBeInTheDocument();
  });
});
