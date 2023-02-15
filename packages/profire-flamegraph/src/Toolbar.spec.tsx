import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Maybe } from 'true-myth';
import Toolbar from './Toolbar';
import { HeadMode, TailMode } from './fitMode/fitMode';

// since 'react-debounce-input' uses lodash.debounce under the hood
jest.mock('lodash.debounce', () =>
  jest.fn((fn) => {
    fn.flush = () => {};
    return fn;
  })
);

describe('ProfileHeader', () => {
  beforeAll(() => {
    window.HTMLElement.prototype.getBoundingClientRect = function () {
      return {
        x: 0,
        y: 0,
        bottom: 0,
        right: 0,
        toJSON: () => {},
        height: 0,
        top: 0,
        left: 0,
        width: 900,
      };
    };
  });

  it('should render toolbar correctly', () => {
    const { asFragment } = render(
      <Toolbar
        view="both"
        flamegraphType="single"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={HeadMode}
        updateView={() => {}}
        isFlamegraphDirty={false}
        selectedNode={Maybe.nothing()}
        onFocusOnSubtree={() => {}}
        highlightQuery=""
      />
    );

    expect(screen.getByRole('toolbar')).toBeInTheDocument();
    expect(asFragment()).toMatchSnapshot();
  });

  describe('Reset button', () => {
    const onReset = jest.fn();

    beforeEach(() => {});

    afterEach(() => {
      jest.clearAllMocks();
    });

    it('renders as disabled when flamegraph is not dirty', () => {
      const component = (
        <Toolbar
          view="both"
          flamegraphType="single"
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(screen.getByRole('button', { name: /Reset/ })).toBeDisabled();
    });

    it('calls onReset when clicked (and enabled)', () => {
      const component = (
        <Toolbar
          view="both"
          flamegraphType="single"
          isFlamegraphDirty
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(screen.getByRole('button', { name: /Reset/ })).not.toBeDisabled();
      screen.getByRole('button', { name: /Reset/ }).click();

      expect(onReset).toHaveBeenCalled();
    });
  });

  describe('HighlightSearch', () => {
    it('calls callback when typed', () => {
      const onChange = jest.fn();

      const component = (
        <Toolbar
          view="both"
          flamegraphType="single"
          isFlamegraphDirty
          handleSearchChange={onChange}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );

      render(component);
      userEvent.type(screen.getByRole('searchbox'), 'foobar');
      expect(onChange).toHaveBeenCalledWith('foobar');
    });
  });

  describe('FitMode', () => {
    const updateFitMode = jest.fn();
    const component = (
      <Toolbar
        view="both"
        flamegraphType="single"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={updateFitMode}
        fitMode={HeadMode}
        updateView={() => {}}
        isFlamegraphDirty={false}
        selectedNode={Maybe.nothing()}
        onFocusOnSubtree={() => {}}
        highlightQuery=""
      />
    );

    beforeEach(() => {
      render(component);
    });

    afterEach(() => {
      jest.clearAllMocks();
    });

    it('updates to HEAD first', () => {
      screen.getByRole('button', { name: 'Head first' }).click();

      expect(updateFitMode).toHaveBeenCalledWith(HeadMode);
    });

    it('updates to TAIL first', () => {
      screen.getByRole('button', { name: 'Tail first' }).click();

      expect(updateFitMode).toHaveBeenCalledWith(TailMode);
    });
  });

  describe('Focus on subtree', () => {
    it('renders as disabled when theres no selected node', () => {
      const component = (
        <Toolbar
          view="both"
          flamegraphType="single"
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(screen.getByRole('button', { name: /Collapse/ })).toBeDisabled();
    });

    it('calls callback when clicked', () => {
      const onFocusOnSubtree = jest.fn();
      const component = (
        <Toolbar
          view="both"
          flamegraphType="single"
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          selectedNode={Maybe.just({ i: 999, j: 999 })}
          onFocusOnSubtree={onFocusOnSubtree}
          highlightQuery=""
        />
      );

      render(component);
      screen.getByRole('button', { name: /Collapse/ }).click();

      expect(onFocusOnSubtree).toHaveBeenCalledWith(999, 999);
    });
  });
});
