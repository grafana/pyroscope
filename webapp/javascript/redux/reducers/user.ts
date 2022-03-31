/* eslint-disable prettier/prettier */
import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import { Users, type User } from '@webapp/models/users';
import { connect } from 'react-redux';

import {
  loadCurrentUser as loadCurrentUserAPI,
  changeMyPassword as changeMyPasswordAPI,
  editMyUser as editMyUserAPI,
} from '@webapp/services/users';
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
    // By using 404 we assume that auth on server is disabled
    // TODO: Fix that
    if ('code' in res.error && res.error.code === 404) {
      return Promise.resolve({ id: 0, role: 'anonymous' });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load current user',
        message: 'Please contact your administrator',
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
      return { type: 'loaded', data: action.payload as User };
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
        title: 'Failed',
        message: 'Failed to change users password',
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const editMe = createAsyncThunk(
  'users/editMyUser',
  async (data: Partial<User>, thunkAPI) => {
    const res = await editMyUserAPI(data);

    if (res.isOk) {
      await thunkAPI.dispatch(loadCurrentUser()).unwrap();
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed',
        message: 'Failed to edit current user',
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const currentUserState = (state: RootState) => state.user;
export const selectCurrentUser = (state: RootState) => state.user?.data;

// TODO: @shaleynikov extract currentUser HOC
// TODO(eh-am): get rid of HOC
export const withCurrentUser = (component: ShamefulAny) =>
  connect((state: RootState) => ({
    currentUser: selectCurrentUser(state),
  }))(function ConditionalRender(props: { currentUser: User }) {
    if (props.currentUser) {
      return component(props);
    }
    return null;
  } as ShamefulAny);

export default userSlice.reducer;
