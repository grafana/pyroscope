import { Target } from '@webapp/models/targets';
import { fetchTargets } from '@webapp/services/serviceDiscovery';
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
      state = { type: 'loaded', data: action.payload };
    });

    builder.addCase(loadTargets.pending, (state) => {
      state = { type: 'loading', data: state.data };
    });
    builder.addCase(loadTargets.rejected, (state) => {
      state = { type: 'failed', data: state.data };
    });
  },
});

export default serviceDiscoverySlice.reducer;
