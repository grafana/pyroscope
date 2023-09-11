import { createSlice } from '@reduxjs/toolkit';
import { type User } from '@pyroscope/models/users';
import { connect, useSelector } from 'react-redux';
import {
  loadCurrentUser as loadCurrentUserAPI,
  changeMyPassword as changeMyPasswordAPI,
  editMyUser as editMyUserAPI,
} from '@pyroscope/services/users';
import type { RootState } from '@pyroscope/redux/store';
import { createAsyncThunk } from '@pyroscope/redux/async-thunk';
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
  'users/loadCurrentUser',
  async (_, thunkAPI) => {
    const res = await loadCurrentUserAPI();
    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    // Suppress 401 error on login screen
    // TODO(petethepig): we need a better way of handling this exception
    if ('code' in res.error && res.error.code === 401) {
      return Promise.reject(res.error);
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
    if (props.currentUser || !(window as ShamefulAny).isAuthRequired) {
      return component(props);
    }
    return null;
  } as ShamefulAny);

export const useCurrentUser = () => useSelector(selectCurrentUser);

export default userSlice.reducer;
