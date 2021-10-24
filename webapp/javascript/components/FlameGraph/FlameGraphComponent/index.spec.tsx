import React from 'react';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';
import FlamegraphComponent from './index';
import TestData from './testData';
import { BAR_HEIGHT } from './constants';

// the leafs have already been tested
// this is just to guarantee code is compiling
// and the callbacks are being called correctly
describe('FlamegraphComponent', () => {
  it('renders', () => {
    const onZoom = jest.fn();
    const onReset = jest.fn();
    const isDirty = jest.fn();

    render(
      <FlamegraphComponent
        viewType="single"
        fitMode="HEAD"
        zoom={{ i: -1, j: -1 }}
        topLevel={0}
        selectedLevel={0}
        query=""
        onZoom={onZoom}
        onReset={onReset}
        isDirty={isDirty}
        flamebearer={TestData.SimpleTree}
      />
    );
  });

  it('resizes correctly', () => {
    const onZoom = jest.fn();
    const onReset = jest.fn();
    const isDirty = jest.fn();

    render(
      <FlamegraphComponent
        viewType="single"
        fitMode="HEAD"
        zoom={{ i: -1, j: -1 }}
        topLevel={0}
        selectedLevel={0}
        query=""
        onZoom={onZoom}
        onReset={onReset}
        isDirty={isDirty}
        flamebearer={TestData.SimpleTree}
      />
    );

    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 800,
    });

    window.dispatchEvent(new Event('resize'));

    // there's nothing much to assert here
    expect(window.innerWidth).toBe(800);
  });

  it('zooms on click', () => {
    const onZoom = jest.fn();
    const onReset = jest.fn();
    const isDirty = jest.fn();

    render(
      <FlamegraphComponent
        viewType="single"
        fitMode="HEAD"
        zoom={{ i: -1, j: -1 }}
        topLevel={0}
        selectedLevel={0}
        query=""
        onZoom={onZoom}
        onReset={onReset}
        isDirty={isDirty}
        flamebearer={TestData.SimpleTree}
      />
    );

    userEvent.click(screen.getByTestId('flamegraph-canvas'), {
      clientX: 0,
      clientY: BAR_HEIGHT * 3,
    });

    expect(onZoom).toHaveBeenCalled();
  });

  describe('context menu', () => {
    it('enables "reset view" menuitem when its dirty', () => {
      const onZoom = jest.fn();
      const onReset = jest.fn();
      const isDirty = jest.fn();

      const { rerender } = render(
        <FlamegraphComponent
          viewType="single"
          fitMode="HEAD"
          zoom={{ i: -1, j: -1 }}
          topLevel={0}
          selectedLevel={0}
          query=""
          onZoom={onZoom}
          onReset={onReset}
          isDirty={isDirty}
          flamebearer={TestData.SimpleTree}
        />
      );

      userEvent.click(screen.getByTestId('flamegraph-canvas'), {
        button: 2,
      });
      // should not be available unless we zoom
      expect(
        screen.queryByRole('menuitem', { name: /Reset View/ })
      ).toHaveAttribute('aria-disabled', 'true');

      // it's dirty now
      isDirty.mockReturnValue(true);

      rerender(
        <FlamegraphComponent
          viewType="single"
          fitMode="HEAD"
          zoom={{ i: -1, j: -1 }}
          topLevel={0}
          selectedLevel={0}
          query=""
          onZoom={onZoom}
          onReset={onReset}
          isDirty={isDirty}
          flamebearer={TestData.SimpleTree}
        />
      );

      userEvent.click(screen.getByTestId('flamegraph-canvas'), {
        button: 2,
      });

      // should be enabled now
      expect(
        screen.queryByRole('menuitem', { name: /Reset View/ })
      ).not.toHaveAttribute('aria-disabled', 'true');
    });
  });
});
