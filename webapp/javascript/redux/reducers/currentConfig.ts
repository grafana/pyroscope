import { CurrentConfig } from '@webapp/models/currentConfig';
import { fetchCurrentConfig } from '@webapp/services/currentConfig';
import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import { addNotification } from './notifications';

export const loadCurrentConfig = createAsyncThunk(
  'curentConfig/loadCurrentConfig',
  async (_, thunkAPI) => {
    const res = await fetchCurrentConfig();

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed',
        message: 'Failed to load config',
      })
    );

    return Promise.reject(res.error);
  }
);

const initialState = { type: 'loaded', data: {} as CurrentConfig };
export const currentConfigSlice = createSlice({
  name: 'currentConfig',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(loadCurrentConfig.fulfilled, (_, action) => {
      return { type: 'loaded', data: action.payload };
    });
    builder.addCase(loadCurrentConfig.pending, (state) => {
      return { type: 'reloading', data: state.data };
    });
    builder.addCase(loadCurrentConfig.rejected, (state) => {
      return { type: 'failed', data: state.data };
    });
  },
});

export default currentConfigSlice.reducer;
