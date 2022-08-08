import React from 'react';
import { Provider } from 'react-redux';
import { render, screen } from '@testing-library/react';
import { configureStore } from '@reduxjs/toolkit';

import continuousReducer from '@webapp/redux/reducers/continuous';
import ViewTagsSelectLinkModal, {
  ViewTagsSelectModalProps,
} from './ViewTagsSelectLinkModal';

const whereDropdownItems = ['foo', 'bar', 'baz'];
const groupByTag = 'groupByTagTest';
const appName = 'appNameTest';
const baselineTag = 'baselineTagTest';
const comparisonTag = 'comparisonTagTest';
const linkName = 'linkNameTest';
const setLinkTagsSelectModalData = jest.fn((v) => v);

function createStore(preloadedState: any) {
  const store = configureStore({
    reducer: {
      continuous: continuousReducer,
    },
    preloadedState,
  });

  return store;
}

describe('Component: ViewTagsSelectLinkModal', () => {
  const renderComponent = (props: ViewTagsSelectModalProps) => {
    render(
      <Provider
        store={createStore({
          continuous: {},
        })}
      >
        <ViewTagsSelectLinkModal {...props} />
      </Provider>
    );
  };

  it('shoudld render ViewTagsSelectLinkModal with default structure', () => {
    renderComponent({
      whereDropdownItems,
      groupByTag,
      appName,
      setLinkTagsSelectModalData,
      baselineTag,
      comparisonTag,
      linkName,
    });

    const modalElement = screen.getByTestId('link-modal');

    // static
    expect(modalElement).toBeInTheDocument();
    expect(
      screen.getByText('Select Tags For linkNameTest')
    ).toBeInTheDocument();
    expect(screen.getByText('baseline')).toBeInTheDocument();
    expect(screen.getByText('comparison')).toBeInTheDocument();
    expect(
      modalElement.querySelector('.modalFooter input')
    ).toBeInTheDocument();
    expect(modalElement.querySelector('.modalFooter input')).toHaveAttribute(
      'value',
      'Compare tags'
    );

    // dynamic
    expect(modalElement.querySelectorAll('.tags')).toHaveLength(2);
    modalElement.querySelectorAll('.tags').forEach((tagList) => {
      tagList.querySelectorAll('input').forEach((tag, i) => {
        expect(tag).toHaveAttribute('value', whereDropdownItems[i]);
      });
    });
  });
});
