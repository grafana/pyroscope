import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ProfileHeader, { TOOLBAR_MODE_WIDTH_THRESHOLD } from './ProfilerHeader';
import { FitModes } from '../util/fitMode';

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
        resetStyle={{}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={() => {}}
        updateViewDiff={() => {}}
      />
    );

    expect(screen.getByRole('toolbar')).toHaveAttribute('data-mode', 'large');
    expect(asFragment()).toMatchSnapshot();

    setWindowSize('small');

    rerender(
      <ProfileHeader
        view="both"
        handleSearchChange={() => {}}
        resetStyle={{}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={() => {}}
        updateViewDiff={() => {}}
      />
    );

    expect(screen.getByRole('toolbar')).toHaveAttribute('data-mode', 'small');
    expect(asFragment()).toMatchSnapshot();
  });

  describe('DiffSection', () => {
    const updateViewDiff = jest.fn();
    const component = (
      <ProfileHeader
        view="both"
        viewDiff="diff"
        handleSearchChange={() => {}}
        resetStyle={{}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={() => {}}
        updateViewDiff={updateViewDiff}
      />
    );

    it('doesnt render if viewDiff is not set', () => {
      render(
        <ProfileHeader
          view="both"
          handleSearchChange={() => {}}
          resetStyle={{}}
          reset={() => {}}
          updateFitMode={() => {}}
          fitMode={FitModes.HEAD}
          updateView={() => {}}
          updateViewDiff={() => {}}
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
        resetStyle={{}}
        reset={() => {}}
        updateFitMode={() => {}}
        fitMode={FitModes.HEAD}
        updateView={updateView}
        updateViewDiff={() => {}}
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
