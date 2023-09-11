import React from 'react';
import { brandQuery } from '../../models/query';
import { appToQuery, App } from '../../models/app';
import {
  render,
  screen,
  fireEvent,
  waitFor,
  act,
} from '@testing-library/react';
import { AppSelector } from './AppSelector';
import { MENU_ITEM_ROLE } from './SelectButton';

jest.mock('@pyroscope/services/apps');

const { getByTestId, queryByRole } = screen;
const mockApps: App[] = [
  { name: 'single', units: 'unknown', spyName: 'unknown' },
  { name: 'double.cpu', units: 'unknown', spyName: 'unknown' },
  { name: 'double.space', units: 'unknown', spyName: 'unknown' },
  { name: 'triple.app.cpu', units: 'unknown', spyName: 'unknown' },
  { name: 'triple.app.objects', units: 'unknown', spyName: 'unknown' },
];

describe('AppSelector', () => {
  describe('when no query exists / is invalid', () => {
    it('renders an empty app selector', () => {
      render(
        <AppSelector
          apps={[]}
          onSelected={() => {}}
          selectedQuery={brandQuery('')}
        />
      );

      expect(screen.getByRole('button')).toHaveTextContent(
        'Select an application'
      );
    });
  });

  describe('when a query exists', () => {
    describe('when an equivalent app exists', () => {
      it('selects that app', () => {
        const apps = [
          {
            __profile_type__: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
            __name_id__: 'pyroscope_app' as const,
            pyroscope_app: 'myapp',
            name: 'myapp',
            __type__: 'type',
            __name__: 'name',
          },
        ];
        const query = appToQuery(apps[0]);

        render(
          <AppSelector
            apps={apps}
            onSelected={() => {}}
            selectedQuery={query}
          />
        );

        expect(screen.getByRole('button')).toHaveTextContent('myapp:name:type');
      });
    });
  });

  describe('when a query exists', () => {
    describe('when an equivalent app DOES NOT exist', () => {
      it('shows the default label', () => {
        const apps = [
          {
            __profile_type__: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
            pyroscope_app: 'myapp',
            __name_id__: 'pyroscope_app' as const,
            name: '',
            __type__: 'type',
            __name__: 'name',
          },
        ];

        const query = brandQuery(
          'memory:alloc_objects:count::1{pyroscope_app="simple.golang.app"}'
        );

        render(
          <AppSelector
            apps={apps}
            onSelected={() => {}}
            selectedQuery={query}
          />
        );

        expect(screen.getByRole('button')).toHaveTextContent(
          'Select an application'
        );
      });
    });
  });

  // TODO: test
  // * interaction
});

describe('AppSelector', () => {
  // TODO copied from og
  it.skip('gets the list of apps, interacts with it -- naming convention changed', async () => {
    const onSelected = jest.fn();

    render(
      <AppSelector apps={mockApps} onSelected={onSelected} selectedAppName="" />
    );

    act(() => getByTestId('toggler').click());

    // checks that there are 3 groups
    expect(queryByRole(MENU_ITEM_ROLE, { name: 'single' })).toBeInTheDocument();
    expect(queryByRole(MENU_ITEM_ROLE, { name: 'double' })).toBeInTheDocument();
    expect(
      queryByRole(MENU_ITEM_ROLE, { name: 'triple.app' })
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: 'single' }));
    expect(onSelected).toHaveBeenCalledWith('single');

    act(() => getByTestId('toggler').click());

    // checks if 'triple' group expands 2 profile types
    fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: 'triple.app' }));
    await waitFor(() => {
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'triple.app.cpu' })
      ).toBeInTheDocument();
      expect(
        queryByRole(MENU_ITEM_ROLE, { name: 'triple.app.objects' })
      ).toBeInTheDocument();
    });
    // checks if 'double' group expands 2 profile types
    fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: 'double' }));
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
  // TODO copied from og
  it.skip('filters apps by query input -- naming conventions changed', async () => {
    const onSelected = jest.fn();

    render(
      <AppSelector apps={mockApps} onSelected={onSelected} selectedAppName="" />
    );

    act(() => getByTestId('toggler').click());

    const input = screen.getByTestId('app-selector-search');
    act(() => fireEvent.change(input, { target: { value: 'triple.app' } }));

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
