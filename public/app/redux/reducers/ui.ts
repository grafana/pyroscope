import { createSlice, createSelector, PayloadAction } from '@reduxjs/toolkit';
import { createMigrate } from 'redux-persist';
import storage from 'redux-persist/lib/storage';
import { PersistedState } from 'redux-persist/lib/types';
import type { RootState } from '@pyroscope/redux/store';

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

export interface UiState {
  time: {
    offset: null | number;
  };
  colorMode: 'dark' | 'light';
}

const initialState: UiState = {
  time: {
    offset: null,
  },
  //  sidebar: { state: 'pristine' },
  colorMode: 'dark',
};

export const uiSlice = createSlice({
  name: 'ui',
  initialState,
  reducers: {
    changeTimeZoneOffset: (state, action) => {
      state.time.offset = action.payload;
    },
    setColorMode: (state, action: PayloadAction<'dark' | 'light'>) => {
      state.colorMode = action.payload;
    },
  },
});

const selectUiState = (state: RootState) => state.ui;

export const selectTimezoneOffset = createSelector(
  selectUiState,
  (state) => state.time.offset
);

export const selectAppColorMode = createSelector(
  selectUiState,
  (state) => state.colorMode
);

export const { changeTimeZoneOffset, setColorMode } = uiSlice.actions;

export default uiSlice.reducer;
