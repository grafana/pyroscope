import { createSlice, createSelector } from '@reduxjs/toolkit';
import { createMigrate } from 'redux-persist';
import storage from 'redux-persist/lib/storage';
import { PersistedState } from 'redux-persist/lib/types';
import type { RootState } from '../store';

// Persistence Migrations
// See examples on https://github.com/rt2zz/redux-persist/blob/master/docs/migrations.md
export const migrations = {
  0: (state: PersistedState) => {
    if (!state) {
      return {} as PersistedState;
    }

    return { ...state };
  },
};

export const persistConfig = {
  key: 'pyroscope:ui',
  version: 0,
  storage,
  migrate: createMigrate(migrations, { debug: true }),
};

type SidebarState =
  // pristine means user hasn't interacted with it yet
  // so we default to certain heuristics (eg window size)
  | { state: 'pristine'; collapsed: true }
  | { state: 'pristine'; collapsed: false }

  // userInteracted means user has actively clicked on the button
  // so we should keep whatever state they've chosen
  | { state: 'userInteracted'; collapsed: true }
  | { state: 'userInteracted'; collapsed: false };

export interface UiState {
  sidebar: SidebarState;
}

const initialState: UiState = {
  sidebar: { state: 'pristine', collapsed: window.innerWidth < 1200 },
  //  sidebar: { state: 'pristine' },
};

export const uiSlice = createSlice({
  name: 'ui',
  initialState,
  reducers: {
    recalculateSidebar: (state) => {
      if (state.sidebar.state === 'pristine') {
        state.sidebar.collapsed = window.innerWidth < 1200;
      }
    },
    collapseSidebar: (state) => {
      state.sidebar = { state: 'userInteracted', collapsed: true };
    },
    uncollapseSidebar: (state) => {
      state.sidebar = { state: 'userInteracted', collapsed: false };
    },
  },
});

const selectUiState = (state: RootState) => state.ui;

export const selectSidebarCollapsed = createSelector(selectUiState, (state) => {
  return state.sidebar.collapsed;
});

export const { collapseSidebar, uncollapseSidebar, recalculateSidebar } =
  uiSlice.actions;

export default uiSlice.reducer;
