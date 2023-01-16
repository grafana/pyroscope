import React from 'react';
import { App } from '@webapp/models/app';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import AppSelector from '.';
import { MENU_ITEM_ROLE } from './SelectButton';

jest.mock('@webapp/services/apps');

const { getByTestId, queryByRole, getByRole, findByRole } = screen;
const mockApps: App[] = [
  { name: 'single', units: 'unknown', spyName: 'unknown' },
  { name: 'double.cpu', units: 'unknown', spyName: 'unknown' },
  { name: 'double.space', units: 'unknown', spyName: 'unknown' },
  { name: 'triple.app.cpu', units: 'unknown', spyName: 'unknown' },
  { name: 'triple.app.objects', units: 'unknown', spyName: 'unknown' },
];

describe('AppSelector', () => {
  it('gets the list of apps, iterracts with it', async () => {
    const onRefresh = jest.fn();
    const onSelected = jest.fn();

    render(
      <AppSelector
        apps={mockApps}
        isLoading={false}
        onRefresh={onRefresh}
        onSelected={onSelected}
        selectedAppName={''}
      />
    );

    getByTestId('toggler').click();

    // checks that there are 3 groups
    expect(queryByRole(MENU_ITEM_ROLE, { name: 'single' })).toBeInTheDocument();
    expect(queryByRole(MENU_ITEM_ROLE, { name: 'double' })).toBeInTheDocument();
    expect(
      queryByRole(MENU_ITEM_ROLE, { name: 'triple.app' })
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: 'single' }));
    expect(onSelected).toHaveBeenCalledWith('single');

    getByTestId('toggler').click();

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
  it('filters apps by query input', async () => {
    const onRefresh = jest.fn();
    const onSelected = jest.fn();

    render(
      <AppSelector
        apps={mockApps}
        isLoading={false}
        onRefresh={onRefresh}
        onSelected={onSelected}
        selectedAppName={''}
      />
    );

    getByTestId('toggler').click();

    const input = screen.getByTestId('application-search');
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
