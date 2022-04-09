import React from 'react';
import { Provider } from 'react-redux';
import { render as rtlRender, screen } from '@testing-library/react';
import { configureStore } from '@reduxjs/toolkit';
import { continuousSlice } from '@webapp/redux/reducers/continuous';
import { Result } from '@webapp/util/fp';
import * as appNames from '@webapp/services/appNames';
import NameSelector from './NameSelector';

jest.mock('@webapp/services/appNames');

function render(
  ui: any,
  {
    store = configureStore({
      reducer: { continuous: continuousSlice.reducer },
    }),
    ...renderOptions
  } = {}
) {
  function Wrapper({ children }: any) {
    return <Provider store={store}>{children}</Provider>;
  }
  return rtlRender(ui, { wrapper: Wrapper, ...renderOptions });
}

describe('NameSelector', () => {
  it('refreshes the list of apps', async () => {
    (appNames as any).fetchAppNames.mockResolvedValueOnce(Result.ok(['myapp']));
    render(<NameSelector />);

    // Initial state
    // the item 'myapp' shouldn't be there
    screen
      .getByRole('button', { name: /Select Application/i, expanded: false })
      .click();
    expect(
      screen.queryByRole('menuitem', { name: 'myapp' })
    ).not.toBeInTheDocument();

    // Refresh
    screen.getByRole('button', { name: /Refresh Apps/i }).click();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();

    // After some time the item should've been loaded
    // and the 'myapp' menuitem should be there
    expect(await screen.findByRole('progressbar')).not.toBeInTheDocument();
    screen
      .getByRole('button', { name: /Select Application/i, expanded: true })
      .click();
    screen.getByRole('menuitem', { name: 'myapp' });
  });
});
