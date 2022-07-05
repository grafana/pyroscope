import React from 'react';
import { Provider } from 'react-redux';
import {
  render as rtlRender,
  screen,
  RenderOptions,
  fireEvent,
  waitFor,
} from '@testing-library/react';
import { configureStore } from '@reduxjs/toolkit';
import { continuousSlice } from '@webapp/redux/reducers/continuous';
import { Result } from '@webapp/util/fp';
import * as appNames from '@webapp/services/appNames';
import AppSelector, { TOGGLE_BTN_ID } from './index';
import { getGroups, APP_SEARCH_INPUT } from './SelectorModal';

jest.mock('@webapp/services/appNames');

const { getByTestId, queryByRole, getByRole, findByRole } = screen;

const MENU_ITEM_ROLE = 'menuitem';
const mockAppNames = [
  'single',
  'double.cpu',
  'double.space',
  'triple.app.cpu',
  'triple.app.objects',
];

interface RenderOpts extends Omit<RenderOptions, 'queries'> {
  preloadedState?: any;
  store?: ReturnType<typeof configureStore>;
}

function render(
  ui: any,
  {
    preloadedState = {},
    store = configureStore({
      reducer: {
        continuous: continuousSlice.reducer,
      },
      preloadedState,
    }),
    ...renderOptions
  }: RenderOpts = {}
) {
  function Wrapper({ children }: any) {
    return <Provider store={store}>{children}</Provider>;
  }
  return rtlRender(ui, { wrapper: Wrapper, ...renderOptions });
}

describe('AppSelector', () => {
  it('refreshes the list of apps', async () => {
    (appNames as any).fetchAppNames.mockResolvedValueOnce(Result.ok(['myapp']));
    render(<AppSelector />);

    // Initial state
    // the item 'myapp' shouldn't be there
    getByTestId(TOGGLE_BTN_ID).click();
    expect(
      queryByRole(MENU_ITEM_ROLE, { name: 'myapp' })
    ).not.toBeInTheDocument();

    // Refresh
    getByRole('button', { name: /Refresh Apps/i }).click();
    expect(getByRole('progressbar')).toBeInTheDocument();

    // After some time the item should've been loaded
    // and the 'myapp' menuitem should be there
    expect(await findByRole('progressbar')).not.toBeInTheDocument();
    getByTestId(TOGGLE_BTN_ID).click();
    getByRole(MENU_ITEM_ROLE, { name: 'myapp' });
  });
});

describe('AppSelector', () => {
  it('gets the list of apps, iterracts with it', async () => {
    (appNames as any).fetchAppNames.mockResolvedValueOnce(Result.ok());

    render(<AppSelector />, {
      preloadedState: {
        continuous: {
          appNames: {
            type: 'loaded',
            data: mockAppNames,
          },
        },
      },
    });

    getByTestId(TOGGLE_BTN_ID).click();

    // splits apps by groups correctly and renders them
    const groups = getGroups(mockAppNames);
    groups.forEach((g) => {
      expect(queryByRole(MENU_ITEM_ROLE, { name: g })).toBeInTheDocument();
    });

    // picks app group with inner profile types
    // expands it
    // has correct list of profile types rendered on DOM
    const trippleAppGroup = groups[2];

    queryByRole(MENU_ITEM_ROLE, { name: trippleAppGroup })?.click();

    const trippleApps = mockAppNames.filter(
      (g) => g.indexOf(trippleAppGroup) !== -1
    );

    trippleApps.forEach((g) => {
      expect(queryByRole(MENU_ITEM_ROLE, { name: g })).toBeInTheDocument();
    });
  });
});

describe('AppSelector', () => {
  it('filters apps by query input', async () => {
    (appNames as any).fetchAppNames.mockResolvedValueOnce(Result.ok());

    const renderUI = render(<AppSelector />, {
      preloadedState: {
        continuous: {
          appNames: {
            type: 'loaded',
            data: mockAppNames,
          },
        },
      },
    });

    getByTestId(TOGGLE_BTN_ID).click();

    // splits apps by groups correctly and renders them
    const groups = getGroups(mockAppNames);
    groups.forEach((name) => {
      expect(queryByRole(MENU_ITEM_ROLE, { name })).toBeInTheDocument();
    });

    // sets triple.app (groups[2]) as input value
    const input = renderUI.getByTestId(APP_SEARCH_INPUT);
    const trippleAppGroup = groups[2];
    fireEvent.change(input, { target: { value: trippleAppGroup } });

    // picks 2 groups, checks either they are in DOM or not
    await waitFor(() => {
      // must be rendered in DOM
      groups
        .filter((g) => g === groups[2])
        .forEach((name) => {
          expect(queryByRole(MENU_ITEM_ROLE, { name })).toBeInTheDocument();
        });
      // mustn't be rendered in DOM
      groups
        .filter((g) => g !== groups[2])
        .forEach((name) => {
          expect(queryByRole(MENU_ITEM_ROLE, { name })).not.toBeInTheDocument();
        });
    });
  });
});
