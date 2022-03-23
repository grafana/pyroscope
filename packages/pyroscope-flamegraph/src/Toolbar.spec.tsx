import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Maybe } from 'true-myth';
import Toolbar, { TOOLBAR_MODE_WIDTH_THRESHOLD } from './Toolbar';
import { HeadMode, TailMode } from './fitMode/fitMode';

// since 'react-debounce-input' uses lodash.debounce under the hood
jest.mock('lodash.debounce', () =>
  jest.fn((fn) => {
    fn.flush = () => {};
    return fn;
  })
);

function setWindowSize(s: 'small' | 'large') {
  const boundingClientRect = {
    x: 0,
    y: 0,
    bottom: 0,
    right: 0,
    toJSON: () => {},
    height: 0,
    top: 0,
    left: 0,
  };

  switch (s) {
    case 'large': {
      // https://github.com/jsdom/jsdom/issues/653#issuecomment-606323844
      window.HTMLElement.prototype.getBoundingClientRect = function () {
        return {
          ...boundingClientRect,
          width: TOOLBAR_MODE_WIDTH_THRESHOLD,
        };
      };
      break;
    }
    case 'small': {
      // https://github.com/jsdom/jsdom/issues/653#issuecomment-606323844
      window.HTMLElement.prototype.getBoundingClientRect = function () {
        return {
          ...boundingClientRect,
          width: TOOLBAR_MODE_WIDTH_THRESHOLD - 1,
        };
      };
      break;
    }

    default: {
      throw new Error('Wrong value');
    }
  }
}

describe('ProfileHeader', () => {
  it('shifts between visualization modes', () => {
    setWindowSize('large');

    const { asFragment, rerender } = render(
      <Toolbar
        view="both"
        viewDiff="diff"
        flamegraphType="single"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={HeadMode}
        updateView={() => {}}
        updateViewDiff={() => {}}
        renderLogo={false}
        isFlamegraphDirty={false}
        selectedNode={Maybe.nothing()}
        onFocusOnSubtree={() => {}}
        highlightQuery=""
      />
    );

    expect(screen.getByRole('toolbar')).toHaveAttribute('data-mode', 'large');
    expect(asFragment()).toMatchSnapshot();

    setWindowSize('small');

    rerender(
      <Toolbar
        view="both"
        viewDiff="diff"
        flamegraphType="single"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={HeadMode}
        updateView={() => {}}
        updateViewDiff={() => {}}
        renderLogo={false}
        isFlamegraphDirty={false}
        selectedNode={Maybe.nothing()}
        onFocusOnSubtree={() => {}}
        highlightQuery=""
      />
    );

    expect(screen.getByRole('toolbar')).toHaveAttribute('data-mode', 'small');
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
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
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
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
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

    it('renders full text when in large screens', () => {
      setWindowSize('large');

      const component = (
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(
        screen.getByRole('button', { name: 'Reset View' })
      ).toBeInTheDocument();
    });

    it('renders short text when in small screens', () => {
      setWindowSize('small');

      const component = (
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(screen.getByRole('button', { name: 'Reset' })).toBeInTheDocument();
    });
  });

  describe('HighlightSearch', () => {
    it('calls callback when typed', () => {
      const onChange = jest.fn();

      const component = (
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty
          handleSearchChange={onChange}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
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
        viewDiff="diff"
        flamegraphType="single"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={updateFitMode}
        fitMode={HeadMode}
        updateView={() => {}}
        updateViewDiff={() => {}}
        renderLogo={false}
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
      userEvent.selectOptions(
        screen.getByRole('combobox', { name: /fit-mode/ }),
        screen.getByRole('option', { name: /Head/ })
      );
      expect(updateFitMode).toHaveBeenCalledWith(HeadMode);
    });

    it('updates to TAIL first', () => {
      userEvent.selectOptions(
        screen.getByRole('combobox', { name: /fit-mode/ }),
        screen.getByRole('option', { name: /Tail/ })
      );

      expect(updateFitMode).toHaveBeenCalledWith(TailMode);
    });
  });

  describe('Focus on subtree', () => {
    it('renders as disabled when theres no selected node', () => {
      const component = (
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(screen.getByRole('button', { name: /Focus/ })).toBeDisabled();
    });

    it('calls callback when clicked', () => {
      const onFocusOnSubtree = jest.fn();
      const component = (
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
          selectedNode={Maybe.just({ i: 999, j: 999 })}
          onFocusOnSubtree={onFocusOnSubtree}
          highlightQuery=""
        />
      );

      render(component);
      screen.getByRole('button', { name: /Focus/ }).click();

      expect(onFocusOnSubtree).toHaveBeenCalledWith(999, 999);
    });

    it('shows short text', () => {
      setWindowSize('small');
      const component = (
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(screen.getByRole('button', { name: 'Focus' })).toBeDisabled();
    });

    it('shows long text', () => {
      setWindowSize('large');
      const component = (
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          renderLogo={false}
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );
      render(component);
      expect(
        screen.getByRole('button', { name: 'Focus on subtree' })
      ).toBeDisabled();
    });
  });

  describe('DiffSection', () => {
    const updateViewDiff = jest.fn();
    const component = (
      <Toolbar
        view="both"
        viewDiff="diff"
        flamegraphType="double"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={HeadMode}
        updateView={() => {}}
        updateViewDiff={updateViewDiff}
        renderLogo={false}
        isFlamegraphDirty={false}
        selectedNode={Maybe.nothing()}
        onFocusOnSubtree={() => {}}
        highlightQuery=""
      />
    );

    it('doesnt render if viewDiff is not set', () => {
      render(
        <Toolbar
          view="both"
          viewDiff="diff"
          flamegraphType="single"
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={HeadMode}
          updateView={() => {}}
          updateViewDiff={() => {}}
          renderLogo={false}
          isFlamegraphDirty={false}
          selectedNode={Maybe.nothing()}
          onFocusOnSubtree={() => {}}
          highlightQuery=""
        />
      );

      expect(screen.queryByTestId('diff-view')).toBeNull();
    });

    describe('large mode', () => {
      beforeEach(() => {
        setWindowSize('large');
        render(component);
      });

      afterEach(() => {
        jest.clearAllMocks();
      });

      it('changes to Self View', () => {
        screen.getByRole('button', { name: /Self/ }).click();
        expect(updateViewDiff).toHaveBeenCalledWith('self');
      });

      it('changes to Total View', () => {
        screen.getByRole('button', { name: /Total/ }).click();
        expect(updateViewDiff).toHaveBeenCalledWith('total');
      });

      it('changes to Diff View', () => {
        screen.getByRole('button', { name: /Diff/ }).click();
        expect(updateViewDiff).toHaveBeenCalledWith('diff');
      });
    });

    describe('small mode', () => {
      beforeEach(() => {
        setWindowSize('small');
        render(component);
      });

      afterEach(() => {
        jest.clearAllMocks();
      });

      it('changes to Self view', () => {
        userEvent.selectOptions(
          screen.getByRole('combobox', { name: /view-diff/ }),
          screen.getByRole('option', { name: /Self/ })
        );
        expect(updateViewDiff).toHaveBeenCalledWith('self');
      });

      it('changes to Total view', () => {
        userEvent.selectOptions(
          screen.getByRole('combobox', { name: /view-diff/ }),
          screen.getByRole('option', { name: /Total/ })
        );
        expect(updateViewDiff).toHaveBeenCalledWith('total');
      });

      it('changes to Diff view', () => {
        userEvent.selectOptions(
          screen.getByRole('combobox', { name: /view-diff/ }),
          screen.getByRole('option', { name: /Diff/ })
        );
        expect(updateViewDiff).toHaveBeenCalledWith('diff');
      });
    });
  });

  describe('ViewSection', () => {
    const updateView = jest.fn();
    const component = (
      <Toolbar
        view="both"
        viewDiff="diff"
        flamegraphType="single"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={HeadMode}
        updateView={updateView}
        updateViewDiff={() => {}}
        renderLogo={false}
        isFlamegraphDirty={false}
        selectedNode={Maybe.nothing()}
        onFocusOnSubtree={() => {}}
        highlightQuery=""
      />
    );

    describe('large mode', () => {
      beforeEach(() => {
        setWindowSize('large');
        render(component);
      });

      afterEach(() => {
        jest.clearAllMocks();
      });

      it('changes to Table View', () => {
        screen.getByRole('button', { name: /Table/ }).click();
        expect(updateView).toHaveBeenCalledWith('table');
      });

      it('changes to Flamegraph view', () => {
        screen.getByRole('button', { name: /Flamegraph/ }).click();
        expect(updateView).toHaveBeenCalledWith('flamegraph');
      });

      it('changes to Both view', () => {
        screen.getByRole('button', { name: /Both/ }).click();
        expect(updateView).toHaveBeenCalledWith('both');
      });
    });

    describe('small mode', () => {
      beforeEach(() => {
        setWindowSize('small');
        render(component);
      });

      afterEach(() => {
        jest.clearAllMocks();
      });

      it('changes to Table view', () => {
        userEvent.selectOptions(
          screen.getByRole('combobox', { name: /view/ }),
          screen.getByRole('option', { name: /Table/ })
        );
        expect(updateView).toHaveBeenCalledWith('table');
      });

      it('changes to Flamegraph view', () => {
        userEvent.selectOptions(
          screen.getByRole('combobox', { name: /view/ }),
          screen.getByRole('option', { name: /Flame/ })
        );
        expect(updateView).toHaveBeenCalledWith('flamegraph');
      });

      it('changes to Both view', () => {
        userEvent.selectOptions(
          screen.getByRole('combobox', { name: /view/ }),
          screen.getByRole('option', { name: /Both/ })
        );
        expect(updateView).toHaveBeenCalledWith('both');
      });
    });
  });
});
