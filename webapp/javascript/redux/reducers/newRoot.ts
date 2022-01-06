import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import { AppNames } from '@models/appNames';
import { Maybe } from '@utils/fp';
import { fetchAppNames } from '@pyroscope/services/appNames';
import type { RootState } from '../store';
import { addNotification } from './notifications';

interface NewRootState {
  // Since the value populated from the server
  // There's no 'loading'
  appNames:
    | { type: 'loaded'; data: AppNames }
    | { type: 'reloading'; data: AppNames }
    | { type: 'failed'; data: AppNames };
}

// Define the initial state using that type
const initialState: NewRootState = {
  // TODO: come from the backend
  appNames: { type: 'loaded', data: (window as any).initialState.appNames },
};

export const reloadAppNames = createAsyncThunk(
  'newRoot/reloadAppNames',
  async (foo, thunkAPI) => {
    // TODO, retries?
    const res = await fetchAppNames();

    res.match({
      Ok: (appNames) => {
        return Promise.resolve(appNames);
      },
      Err: (e) => {
        thunkAPI.dispatch(
          addNotification({
            type: 'danger',
            title: 'Failed to load app names',
            message: e.message,
          })
        );

        return Promise.reject(e);
      },
    });
  }
);

export const newRootSlice = createSlice({
  name: 'newRoot',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadAppNames.fulfilled, (state) => {
      state.appNames = { type: 'loaded', data: state.appNames.data };
    });
    builder.addCase(reloadAppNames.pending, (state) => {
      state.appNames = { type: 'reloading', data: state.appNames.data };
    });
    builder.addCase(reloadAppNames.rejected, (state) => {
      state.appNames = { type: 'failed', data: state.appNames.data };
    });
  },
});

// TODO use maybe here
export const selectAppNamesState = (state: RootState) => state.newRoot.appNames;
export const selectAppNames = (state: RootState) => {
  switch (state.newRoot.appNames.type) {
    case 'loaded':
    case 'reloading': {
      return Maybe.just(state.newRoot.appNames.data);
    }

    default:
      return Maybe.nothing<AppNames>();
  }
};

export default newRootSlice.reducer;
