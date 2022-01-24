import React from 'react';
import { Provider } from 'react-redux';
import { render as rtlRender, screen, waitFor } from '@testing-library/react';
import { configureStore } from '@reduxjs/toolkit';
import { newRootSlice } from '@pyroscope/redux/reducers/newRoot';
import { Result } from '@utils/fp';
import * as appNames from '@pyroscope/services/appNames';
import NameSelector from './NameSelector';

jest.mock('@pyroscope/services/appNames');

function render(
  ui,
  {
    store = configureStore({
      reducer: { newRoot: newRootSlice.reducer },
    }),
    ...renderOptions
  } = {}
) {
  function Wrapper({ children }) {
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

  //  it('sets the first available app as the default', () => {
  //    (appNames as any).fetchAppNames.mockResolvedValueOnce(Result.ok(['myapp']));
  //    render(<NameSelector />);
  //  })
});
