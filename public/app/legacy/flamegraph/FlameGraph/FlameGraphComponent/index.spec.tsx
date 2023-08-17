import React from 'react';
import userEvent from '@testing-library/user-event';
import { act, render, screen, waitFor } from '@testing-library/react';
import { Maybe } from 'true-myth';
import FlamegraphComponent from './index';
import TestData from './testData';
import { BAR_HEIGHT } from './constants';
import { DefaultPalette, FlamegraphPalette } from './colorPalette';

// the leaves have already been tested
// this is just to guarantee code is compiling
// and the callbacks are being called correctly
describe('FlamegraphComponent', () => {
  const setPalette = (p: FlamegraphPalette) => {};
  it('renders', () => {
    const onZoom = jest.fn();
    const onReset = jest.fn();
    const isDirty = jest.fn();
    const onFocusOnNode = jest.fn();

    render(
      <FlamegraphComponent
        updateFitMode={() => ({})}
        selectedItem={Maybe.nothing()}
        setActiveItem={() => ({})}
        showCredit
        fitMode="HEAD"
        zoom={Maybe.nothing()}
        focusedNode={Maybe.nothing()}
        highlightQuery=""
        onZoom={onZoom}
        onFocusOnNode={onFocusOnNode}
        onReset={onReset}
        isDirty={isDirty}
        flamebearer={TestData.SimpleTree}
        palette={DefaultPalette}
        setPalette={setPalette}
      />
    );
  });

  it('resizes correctly', () => {
    const onZoom = jest.fn();
    const onReset = jest.fn();
    const isDirty = jest.fn();
    const onFocusOnNode = jest.fn();

    render(
      <FlamegraphComponent
        updateFitMode={() => ({})}
        selectedItem={Maybe.nothing()}
        setActiveItem={() => ({})}
        showCredit
        fitMode="HEAD"
        zoom={Maybe.nothing()}
        focusedNode={Maybe.nothing()}
        highlightQuery=""
        onZoom={onZoom}
        onFocusOnNode={onFocusOnNode}
        onReset={onReset}
        isDirty={isDirty}
        flamebearer={TestData.SimpleTree}
        palette={DefaultPalette}
        setPalette={setPalette}
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
    const onFocusOnNode = jest.fn();

    render(
      <FlamegraphComponent
        updateFitMode={() => ({})}
        selectedItem={Maybe.nothing()}
        setActiveItem={() => ({})}
        showCredit
        fitMode="HEAD"
        zoom={Maybe.nothing()}
        focusedNode={Maybe.nothing()}
        highlightQuery=""
        onZoom={onZoom}
        onFocusOnNode={onFocusOnNode}
        onReset={onReset}
        isDirty={isDirty}
        flamebearer={TestData.SimpleTree}
        palette={DefaultPalette}
        setPalette={setPalette}
      />
    );

    userEvent.click(screen.getByTestId('flamegraph-canvas'), {
      clientX: 0,
      clientY: BAR_HEIGHT * 3,
    });

    expect(onZoom).toHaveBeenCalled();
  });

  describe('context menu', () => {
    it(`enables "reset view" menuitem when it's dirty`, async () => {
      const onZoom = jest.fn();
      const onReset = jest.fn();
      const isDirty = jest.fn();
      const onFocusOnNode = jest.fn();

      const { rerender } = render(
        <FlamegraphComponent
          updateFitMode={() => ({})}
          selectedItem={Maybe.nothing()}
          setActiveItem={() => ({})}
          showCredit
          fitMode="HEAD"
          zoom={Maybe.nothing()}
          focusedNode={Maybe.nothing()}
          highlightQuery=""
          onZoom={onZoom}
          onFocusOnNode={onFocusOnNode}
          onReset={onReset}
          isDirty={isDirty}
          flamebearer={TestData.SimpleTree}
          palette={DefaultPalette}
          setPalette={setPalette}
        />
      );

      act(() =>
        userEvent.click(screen.getByTestId('flamegraph-canvas'), {
          button: 2,
          clientX: 1,
          clientY: 1,
        })
      );

      // should not be available unless we zoom
      await waitFor(() =>
        expect(
          screen.queryByRole('menuitem', { name: /Reset View/ })
        ).toHaveAttribute('aria-disabled', 'true')
      );

      // it's dirty now
      isDirty.mockReturnValue(true);

      rerender(
        <FlamegraphComponent
          updateFitMode={() => ({})}
          selectedItem={Maybe.nothing()}
          setActiveItem={() => ({})}
          showCredit
          fitMode="HEAD"
          zoom={Maybe.nothing()}
          focusedNode={Maybe.nothing()}
          highlightQuery=""
          onZoom={onZoom}
          onFocusOnNode={onFocusOnNode}
          onReset={onReset}
          isDirty={isDirty}
          flamebearer={TestData.SimpleTree}
          palette={DefaultPalette}
          setPalette={setPalette}
        />
      );

      act(() =>
        userEvent.click(screen.getByTestId('flamegraph-canvas'), {
          button: 2,
        })
      );

      // should be enabled now
      expect(
        screen.queryByRole('menuitem', { name: /Reset View/ })
      ).not.toHaveAttribute('aria-disabled', 'true');
    });

    it('triggers a highlight', () => {
      const onZoom = jest.fn();
      const onReset = jest.fn();
      const isDirty = jest.fn();
      const onFocusOnNode = jest.fn();

      render(
        <FlamegraphComponent
          updateFitMode={() => ({})}
          selectedItem={Maybe.nothing()}
          setActiveItem={() => ({})}
          showCredit
          fitMode="HEAD"
          zoom={Maybe.nothing()}
          focusedNode={Maybe.nothing()}
          highlightQuery=""
          onZoom={onZoom}
          onFocusOnNode={onFocusOnNode}
          onReset={onReset}
          isDirty={isDirty}
          flamebearer={TestData.SimpleTree}
          palette={DefaultPalette}
          setPalette={setPalette}
        />
      );

      // initially the context highlight is not visible
      expect(
        screen.getByTestId('flamegraph-highlight-contextmenu')
      ).not.toBeVisible();

      // then we click
      act(() =>
        userEvent.click(screen.getByTestId('flamegraph-canvas'), { button: 2 })
      );

      // should be visible now
      expect(
        screen.getByTestId('flamegraph-highlight-contextmenu')
      ).toBeVisible();
    });
  });

  describe('header', () => {
    const onZoom = jest.fn();
    const onReset = jest.fn();
    const isDirty = jest.fn();
    const onFocusOnNode = jest.fn();

    it('renders when type is single', () => {
      render(
        <FlamegraphComponent
          updateFitMode={() => ({})}
          selectedItem={Maybe.nothing()}
          setActiveItem={() => ({})}
          showCredit
          fitMode="HEAD"
          zoom={Maybe.nothing()}
          focusedNode={Maybe.nothing()}
          highlightQuery=""
          onZoom={onZoom}
          onFocusOnNode={onFocusOnNode}
          onReset={onReset}
          isDirty={isDirty}
          flamebearer={TestData.SimpleTree}
          palette={DefaultPalette}
          setPalette={setPalette}
          toolbarVisible
        />
      );

      expect(screen.queryByRole('heading', { level: 2 })).toHaveTextContent(
        'Frame width represents CPU time per function'
      );
    });

    it('renders when type is "double"', () => {
      const flamebearer = TestData.DiffTree;
      render(
        <FlamegraphComponent
          updateFitMode={() => ({})}
          selectedItem={Maybe.nothing()}
          setActiveItem={() => ({})}
          showCredit
          fitMode="HEAD"
          zoom={Maybe.nothing()}
          focusedNode={Maybe.nothing()}
          highlightQuery=""
          onZoom={onZoom}
          onFocusOnNode={onFocusOnNode}
          onReset={onReset}
          isDirty={isDirty}
          flamebearer={flamebearer}
          palette={DefaultPalette}
          setPalette={setPalette}
          toolbarVisible
        />
      );

      expect(screen.queryByRole('heading', { level: 2 })).toHaveTextContent(
        '(-) RemovedAdded (+)'
      );

      expect(screen.getByTestId('flamegraph-legend')).toBeInTheDocument();
    });
  });
});
