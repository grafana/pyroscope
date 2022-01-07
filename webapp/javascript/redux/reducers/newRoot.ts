import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import { AppNames } from '@models/appNames';
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
  appNames: { type: 'loaded', data: (window as any).initialState.appNames },
};

export const reloadAppNames = createAsyncThunk(
  'newRoot/reloadAppNames',
  async (foo, thunkAPI) => {
    // TODO, retries?
    const res = await fetchAppNames();

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load app names',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const newRootSlice = createSlice({
  name: 'newRoot',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadAppNames.fulfilled, (state, action) => {
      state.appNames = { type: 'loaded', data: action.payload };
    });
    builder.addCase(reloadAppNames.pending, (state) => {
      state.appNames = { type: 'reloading', data: state.appNames.data };
    });
    builder.addCase(reloadAppNames.rejected, (state) => {
      state.appNames = { type: 'failed', data: state.appNames.data };
    });
  },
});

export const selectAppNamesState = (state: RootState) => state.newRoot.appNames;
export const selectAppNames = (state: RootState) => state.newRoot.appNames.data;

export default newRootSlice.reducer;
