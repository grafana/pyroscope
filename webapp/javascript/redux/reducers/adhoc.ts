import { Profile } from '@pyroscope/models/src';
import { createAsyncThunk, createSlice } from '@reduxjs/toolkit';
import { upload } from '@webapp/services/adhoc';
import { addNotification } from './notifications';

type SingleView =
  | { type: 'pristine' }
  | { type: 'loading' }
  | { type: 'loaded'; profile: Profile }
  | { type: 'reloading'; profile: Profile };

interface AdhocState {
  singleView: SingleView;
}

const initialState: AdhocState = {
  singleView: { type: 'pristine' },
};

export const uploadFile = createAsyncThunk(
  'adhoc/uploadFile',
  async (file: File, thunkAPI) => {
    const res = await upload(file);

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to upload adhoc file',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const adhocSlice = createSlice({
  name: 'adhoc',
  initialState,
  reducers: {},
});
