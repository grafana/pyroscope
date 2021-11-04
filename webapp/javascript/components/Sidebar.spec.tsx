import React from 'react';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { ShortcutProvider } from 'react-keybind';
import Sidebar from './Sidebar';
import history from '../util/history';

jest.mock('react-redux', () => ({
  connect: () => (Component: any) => Component,
}));

describe('Sidebar', () => {
  it('Successfully changes location when clicked', () => {
    render(
      <ShortcutProvider>
        <Sidebar />
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
  it('Successfully changes location when it changes externally', () => {
    // Replace the listener implementation with a mock.
    let registeredListener;
    history.listen = (fn) => {
      registeredListener = fn;
      return () => {};
    };

    render(
      <ShortcutProvider>
        <Sidebar />
      </ShortcutProvider>
    );
    expect(registeredListener).not.toBeUndefined();
    const singleView = screen.getByTestId('sidebar-root');
    const comparisonView = screen.getByTestId('sidebar-comparison');
    fireEvent.click(comparisonView);
    fireEvent.click(singleView);
    expect(singleView).toHaveClass('active-route');
    expect(comparisonView).not.toHaveClass('active-route');

    // Changes the location on the history object
    act(() => {
      registeredListener({ pathname: '/comparison' });
    });
    expect(singleView).not.toHaveClass('active-route');
    expect(comparisonView).toHaveClass('active-route');
  });
});
