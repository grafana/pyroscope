import React from 'react';
import { Provider } from 'react-redux';
import { act, render, screen } from '@testing-library/react';
import { configureStore } from '@reduxjs/toolkit';
import { BrowserRouter } from 'react-router-dom';

import { continuousReducer } from '../../../redux/reducers/continuous';
import TagsSelector, { TagSelectorProps } from './TagsSelector';
import { setStore } from '@pyroscope/services/storage';
const whereDropdownItems = ['foo', 'bar', 'baz'];
const groupByTag = 'group-by-tag-test';
const appName = 'app-name-test';
const linkName = 'link-name-test';

function createStore(preloadedState: any) {
  const store = configureStore({
    reducer: {
      continuous: continuousReducer,
    },
    preloadedState,
  });
  setStore(store);
  return store;
}

describe('Component: ViewTagsSelectLinkModal', () => {
  const renderComponent = (props: TagSelectorProps) => {
    render(
      <Provider
        store={createStore({
          continuous: {},
        })}
      >
        <TagsSelector {...props} />
      </Provider>,
      // https://github.com/testing-library/react-testing-library/issues/972
      { wrapper: BrowserRouter as ShamefulAny }
    );
  };

  it('shoudld successfully render ModalWithToggle', () => {
    renderComponent({
      appName,
      groupByTag,
      linkName,
      whereDropdownItems,
    });

    // triggers click
    act(() => screen.getByTestId('toggler').click());
    const modalWithToggleEl = screen.getByTestId('modal');

    expect(modalWithToggleEl).toBeInTheDocument();

    // static
    expect(
      screen.getByText('Select Tags For link-name-test')
    ).toBeInTheDocument();
    expect(screen.getByText('baseline')).toBeInTheDocument();
    expect(screen.getByText('comparison')).toBeInTheDocument();
    expect(
      modalWithToggleEl.querySelector('.modalFooter input')
    ).toBeInTheDocument();
    expect(
      modalWithToggleEl.querySelector('.modalFooter input')
    ).toHaveAttribute('value', 'Compare tags');

    // dynamic
    expect(modalWithToggleEl.querySelectorAll('.tags')).toHaveLength(2);
    modalWithToggleEl.querySelectorAll('.tags').forEach((tagList) => {
      tagList.querySelectorAll('input').forEach((tag, i) => {
        expect(tag).toHaveAttribute('value', whereDropdownItems[i]);
        act(() => tag.click());
        expect(tag.parentElement).toHaveClass('selected');
      });
    });

    // second click
    act(() => screen.getByTestId('toggler').click());
    expect(modalWithToggleEl).not.toBeInTheDocument();
  });
});
