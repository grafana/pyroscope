import { fetchTargets } from '@pyroscope/services/serviceDiscovery';
import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import { addNotification } from './notifications';

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
        title: 'Failed',
        message: 'Failed to load targets',
      })
    );

    return Promise.reject(res.error);
  }
);

const initialState = { type: 'loaded', data: [] };
export const serviceDiscoverySlice = createSlice({
  name: 'serviceDiscovery',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(loadTargets.fulfilled, (_, action) => {
      return { type: 'loaded', data: action.payload };
    });
    builder.addCase(loadTargets.pending, (state) => {
      return { type: 'reloading', data: state.data };
    });
    builder.addCase(loadTargets.rejected, (state) => {
      return { type: 'failed', data: state.data };
    });
  },
});

export default serviceDiscoverySlice.reducer;
