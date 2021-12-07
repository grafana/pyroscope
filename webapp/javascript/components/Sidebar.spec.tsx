import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { render, screen, fireEvent } from '@testing-library/react';
import { ShortcutProvider } from 'react-keybind';
import Sidebar from './Sidebar2';

jest.mock('react-redux', () => ({
  connect: () => (Component: any) => Component,
}));

describe('Sidebar', () => {
  describe('active routes highlight', () => {
    describe.each([
      ['/', 'sidebar-continuous-single'],
      ['/comparison', 'sidebar-continuous-comparison'],
      ['/comparison-diff', 'sidebar-continuous-diff'],
    ])('visiting route %s', (a, b) => {
      describe('collapsed', () => {
        test(`should have menuitem ${b} active`, () => {
          render(
            <MemoryRouter initialEntries={[a]}>
              <Sidebar initialCollapsed />
            </MemoryRouter>
          );

          // it should be active
          expect(screen.getByTestId(b)).toHaveClass('active');
        });
      });

      describe('not collapsed', () => {
        test(`should have menuitem ${b} active`, () => {
          render(
            <MemoryRouter initialEntries={[a]}>
              <Sidebar initialCollapsed={false} />
            </MemoryRouter>
          );

          // it should be active
          expect(screen.getByTestId(b)).toHaveClass('active');
        });
      });
    });
  });
});
