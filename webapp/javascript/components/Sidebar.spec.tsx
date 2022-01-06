import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import { Sidebar } from './Sidebar';

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
              <Sidebar collapsed />
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
              <Sidebar collapsed={false} />
            </MemoryRouter>
          );

          // it should be active
          expect(screen.getByTestId(b)).toHaveClass('active');
        });
      });
    });
  });
});
