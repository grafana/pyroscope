/* eslint-disable prettier/prettier */
import {
  createSlice,
  createAsyncThunk,
  combineReducers,
} from '@reduxjs/toolkit';
import { Users, type User } from '@models/users';

import {
  loadCurrentUser as loadCurrentUserAPI,
  changeMyPassword as changeMyPasswordAPI,
} from '@pyroscope/services/users';
import type { RootState } from '../store';
import { addNotification } from './notifications';

interface UserRootState {
  type: 'loading' | 'loaded' | 'failed';
  data?: User;
}

// Define the initial state using that type
const initialState: UserRootState = {
  type: 'loading',
  data: undefined,
};

export const loadCurrentUser = createAsyncThunk(
  'newRoot/loadCurrentUser',
  async (_, thunkAPI) => {
    const res = await loadCurrentUserAPI();
    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load current user',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

const userSlice = createSlice({
  name: 'user',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(loadCurrentUser.fulfilled, (state, action) => {
      return { type: 'loaded', data: action.payload };
    });
    builder.addCase(loadCurrentUser.pending, (state) => {
      return { type: 'loading', data: state.data };
    });
    builder.addCase(loadCurrentUser.rejected, (state) => {
      return { type: 'failed', data: state.data };
    });
  },
});

export const changeMyPassword = createAsyncThunk(
  'users/changeMyPassword',
  async (passwords: { oldPassword: string; newPassword: string }, thunkAPI) => {
    const res = await changeMyPasswordAPI(
      passwords.oldPassword,
      passwords.newPassword
    );

    if (res.isOk) {
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to change users password',
        message: res.error.message,
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const currentUserState = (state: RootState) => state.user;
export const selectCurrentUser = (state: RootState) => state.user.data;

export default userSlice.reducer;
