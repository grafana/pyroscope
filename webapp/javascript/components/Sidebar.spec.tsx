import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { render, screen, fireEvent } from '@testing-library/react';
import { ShortcutProvider } from 'react-keybind';
import Sidebar from './Sidebar';

jest.mock('react-redux', () => ({
  connect: () => (Component: any) => Component,
}));

describe('Sidebar', () => {
  it('Successfully changes location when clicked', () => {
    render(
      <ShortcutProvider>
        <MemoryRouter>
          <Sidebar />
        </MemoryRouter>
      </ShortcutProvider>
    );
    const singleView = screen.getByTestId('sidebar-root');
    const comparisonView = screen.getByTestId('sidebar-comparison');
    expect(singleView).toHaveClass('active-route');
    expect(comparisonView).not.toHaveClass('active-route');
    fireEvent.click(comparisonView);
    expect(singleView).not.toHaveClass('active-route');
    expect(comparisonView).toHaveClass('active-route');
    fireEvent.click(singleView);
    expect(singleView).toHaveClass('active-route');
    expect(comparisonView).not.toHaveClass('active-route');
  });
});
