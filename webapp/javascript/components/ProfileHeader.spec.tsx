import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import ProfileHeader, { TOOLBAR_MODE_WIDTH_THRESHOLD } from './ProfilerHeader';
import { FitModes } from '../util/fitMode';

describe('ProfileHeader', () => {
  const elementWidth = jest.fn();

  beforeAll(() => {});
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

  it('shifts between visualization modes', () => {
    // https://github.com/jsdom/jsdom/issues/653#issuecomment-606323844
    window.HTMLElement.prototype.getBoundingClientRect = function () {
      return {
        ...boundingClientRect,
        width: TOOLBAR_MODE_WIDTH_THRESHOLD,
      };
    };

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

    // small mode
    window.HTMLElement.prototype.getBoundingClientRect = function () {
      return {
        ...boundingClientRect,
        width: TOOLBAR_MODE_WIDTH_THRESHOLD - 1,
      };
    };

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
});
