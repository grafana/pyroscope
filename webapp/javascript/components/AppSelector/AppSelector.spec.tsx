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
import * as apps from '@webapp/services/apps';
import { App } from '@webapp/models/app';
import AppSelector from '.';
import { MENU_ITEM_ROLE } from './SelectButton';

jest.mock('@webapp/services/apps');

const fetchAppsMock = apps.fetchApps as jest.MockedFunction<
  typeof apps.fetchApps
>;

const { getByTestId, queryByRole, getByRole, findByRole } = screen;
const mockApps: App[] = [
  { name: 'single', units: 'unknown', spyName: 'unknown' },
  { name: 'double.cpu', units: 'unknown', spyName: 'unknown' },
  { name: 'double.space', units: 'unknown', spyName: 'unknown' },
  { name: 'triple.app.cpu', units: 'unknown', spyName: 'unknown' },
  { name: 'triple.app.objects', units: 'unknown', spyName: 'unknown' },
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
    fetchAppsMock.mockResolvedValueOnce(
      Result.ok([{ name: 'myapp', units: 'unknown', spyName: 'unknown' }])
    );
    render(<AppSelector />);

    // Initial state
    // the item 'myapp' shouldn't be there
    getByTestId('toggler').click();
    expect(
      queryByRole(MENU_ITEM_ROLE, { name: 'myapp' })
    ).not.toBeInTheDocument();

    // Refresh
    getByRole('button', { name: /Refresh Apps/i }).click();
    expect(getByRole('progressbar')).toBeInTheDocument();

    // After some time the item should've been loaded
    // and the 'myapp' menuitem should be there
    expect(await findByRole('progressbar')).not.toBeInTheDocument();
    getByRole(MENU_ITEM_ROLE, { name: 'myapp' });
  });
});

describe('AppSelector', () => {
  it('gets the list of apps, iterracts with it', async () => {
    fetchAppsMock.mockResolvedValueOnce(Result.ok(mockApps));

    const ui = render(<AppSelector />, {
      preloadedState: {
        continuous: {
          apps: {
            type: 'loaded',
            data: mockApps,
          },
          tagExplorerView: {
            groupByTag: '',
            groupByTagValue: '',
            type: 'pristine',
            groups: {},
            timeline: {
              startTime: 0,
              samples: [],
              durationDelta: 0,
            },
          },
        },
      },
    });

    getByTestId('toggler').click();

    // checks that there are 3 groups
    expect(queryByRole(MENU_ITEM_ROLE, { name: 'single' })).toBeInTheDocument();
    expect(queryByRole(MENU_ITEM_ROLE, { name: 'double' })).toBeInTheDocument();
    expect(
      queryByRole(MENU_ITEM_ROLE, { name: 'triple.app' })
    ).toBeInTheDocument();

    // checks if 'single' group is really sigle
    // what means that after click on this elem it propagates
    // as content of toggle button
    const singleGroupName = 'single';
    fireEvent.click(ui.getByRole(MENU_ITEM_ROLE, { name: singleGroupName }));
    await waitFor(() => {
      expect(getByTestId('toggler')).toHaveTextContent(singleGroupName);
    });

    getByTestId('toggler').click();

    // checks if 'tripple' group expands 2 profile types
    fireEvent.click(ui.getByRole(MENU_ITEM_ROLE, { name: 'triple.app' }));
    await waitFor(() => {
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'triple.app.cpu' })
      ).toBeInTheDocument();
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'triple.app.objects' })
      ).toBeInTheDocument();
    });
    // checks if 'double' group expands 2 profile types
    fireEvent.click(ui.getByRole(MENU_ITEM_ROLE, { name: 'double' }));
    await waitFor(() => {
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'double.space' })
      ).toBeInTheDocument();
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'double.cpu' })
      ).toBeInTheDocument();
    });
  });
});

describe('AppSelector', () => {
  it('filters apps by query input', async () => {
    fetchAppsMock.mockResolvedValueOnce(Result.ok(mockApps));

    const renderUI = render(<AppSelector />, {
      preloadedState: {
        continuous: {
          apps: {
            type: 'loaded',
            data: mockApps,
          },
        },
      },
    });

    getByTestId('toggler').click();

    const input = renderUI.getByTestId('application-search');
    fireEvent.change(input, { target: { value: 'triple.app' } });

    // picks groups, which either should be rendered or not
    await waitFor(() => {
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'single' })
      ).not.toBeInTheDocument();
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'double' })
      ).not.toBeInTheDocument();
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'triple.app' })
      ).toBeInTheDocument();
    });
  });
});
