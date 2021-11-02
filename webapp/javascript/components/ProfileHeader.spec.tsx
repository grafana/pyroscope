import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import lodash from 'lodash';
import ProfileHeader, { TOOLBAR_MODE_WIDTH_THRESHOLD } from './ProfilerHeader';
import { FitModes } from '../util/fitMode';

lodash.debounce = jest.fn((fn) => fn) as any;

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
      <ProfileHeader
        view="both"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={() => {}}
        updateViewDiff={() => {}}
        isFlamegraphDirty={false}
      />
    );

    expect(screen.getByRole('toolbar')).toHaveAttribute('data-mode', 'large');
    expect(asFragment()).toMatchSnapshot();

    setWindowSize('small');

    rerender(
      <ProfileHeader
        view="both"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={() => {}}
        updateViewDiff={() => {}}
        isFlamegraphDirty={false}
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
        <ProfileHeader
          view="both"
          viewDiff="diff"
          isFlamegraphDirty={false}
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={FitModes.HEAD}
          updateView={() => {}}
          updateViewDiff={() => {}}
        />
      );
      render(component);
      expect(screen.getByRole('button', { name: /Reset View/ })).toBeDisabled();
    });

    it('calls onReset when clicked (and enabled)', () => {
      const component = (
        <ProfileHeader
          view="both"
          viewDiff="diff"
          isFlamegraphDirty
          handleSearchChange={() => {}}
          reset={onReset}
          updateFitMode={() => {}}
          fitMode={FitModes.HEAD}
          updateView={() => {}}
          updateViewDiff={() => {}}
        />
      );
      render(component);
      expect(
        screen.getByRole('button', { name: /Reset View/ })
      ).not.toBeDisabled();
      screen.getByRole('button', { name: /Reset View/ }).click();

      expect(onReset).toHaveBeenCalled();
    });
  });

  describe('HighlightSearch', () => {
    it('calls callback when typed', () => {
      const onChange = jest.fn();

      const component = (
        <ProfileHeader
          view="both"
          viewDiff="diff"
          isFlamegraphDirty
          handleSearchChange={onChange}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={FitModes.HEAD}
          updateView={() => {}}
          updateViewDiff={() => {}}
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
      <ProfileHeader
        view="both"
        viewDiff="diff"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={updateFitMode}
        fitMode={FitModes.HEAD}
        updateView={() => {}}
        updateViewDiff={() => {}}
        isFlamegraphDirty={false}
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
      expect(updateFitMode).toHaveBeenCalledWith(FitModes.HEAD);
    });

    it('updates to TAIL first', () => {
      userEvent.selectOptions(
        screen.getByRole('combobox', { name: /fit-mode/ }),
        screen.getByRole('option', { name: /Tail/ })
      );

      expect(updateFitMode).toHaveBeenCalledWith(FitModes.TAIL);
    });
  });

  describe('DiffSection', () => {
    const updateViewDiff = jest.fn();
    const component = (
      <ProfileHeader
        view="both"
        viewDiff="diff"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={() => {}}
        updateViewDiff={updateViewDiff}
        isFlamegraphDirty={false}
      />
    );

    it('doesnt render if viewDiff is not set', () => {
      render(
        <ProfileHeader
          view="both"
          handleSearchChange={() => {}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={FitModes.HEAD}
          updateView={() => {}}
          updateViewDiff={() => {}}
          isFlamegraphDirty={false}
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
      <ProfileHeader
        view="both"
        handleSearchChange={() => {}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={updateView}
        updateViewDiff={() => {}}
        isFlamegraphDirty={false}
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
        expect(updateView).toHaveBeenCalledWith('icicle');
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
          screen.getByRole('option', { name: /Flamegraph/ })
        );
        expect(updateView).toHaveBeenCalledWith('icicle');
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
