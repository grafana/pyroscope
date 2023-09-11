import { Target } from '@pyroscope/models/targets';
import { fetchTargets } from '@pyroscope/services/serviceDiscovery';
import { createSlice } from '@reduxjs/toolkit';
import { addNotification } from './notifications';
import type { RootState } from '../store';
import { createAsyncThunk } from '../async-thunk';

export const loadTargets = createAsyncThunk(
  'serviceDiscovery/loadTargets',
  async (_, thunkAPI) => {
    const res = await fetchTargets();

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load targets',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

interface State {
  type: 'pristine' | 'loading' | 'failed' | 'loaded';
  data: Target[];
}
const initialState: State = { type: 'loaded', data: [] };

export const serviceDiscoverySlice = createSlice({
  name: 'serviceDiscovery',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(loadTargets.fulfilled, (state, action) => {
      state.data = action.payload;
      state.type = 'loaded';
    });

    builder.addCase(loadTargets.pending, (state) => {
      state.type = 'loading';
    });
    builder.addCase(loadTargets.rejected, (state) => {
      state.type = 'failed';
    });
  },
});

export default serviceDiscoverySlice.reducer;

export function selectTargetsData(s: RootState) {
  return s.serviceDiscovery.data;
}
