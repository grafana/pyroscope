import React from 'react';
import { brandQuery } from '@pyroscope/models/query';
import { render, screen, fireEvent } from '@testing-library/react';

import QueryInput from './QueryInput';

describe('QueryInput', () => {
  it('changes content correctly', () => {
    const onSubmit = jest.fn();
    render(
      <QueryInput initialQuery={brandQuery('myquery')} onSubmit={onSubmit} />
    );

    const form = screen.getByRole('form', { name: /query-input/i });
    fireEvent.submit(form);
    expect(onSubmit).toHaveBeenCalledWith('myquery');

    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'myquery2' } });
    fireEvent.submit(form);
    expect(onSubmit).toHaveBeenCalledWith('myquery2');
  });

  describe('submission', () => {
    const onSubmit = jest.fn();

    beforeEach(() => {
      render(
        <QueryInput initialQuery={brandQuery('myquery')} onSubmit={onSubmit} />
      );
    });

    it('is submitted by pressing Enter', () => {
      const input = screen.getByRole('textbox');
      fireEvent.keyDown(input, { key: 'Enter' });
      expect(onSubmit).toHaveBeenCalledWith('myquery');
    });

    it('is submitted by clicking on the Execute button', () => {
      const button = screen.getByRole('button');
      button.click();
      expect(onSubmit).toHaveBeenCalledWith('myquery');
    });
  });
});
