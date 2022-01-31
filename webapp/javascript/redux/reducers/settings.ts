import {
  createSlice,
  createAsyncThunk,
  combineReducers,
} from '@reduxjs/toolkit';
import { Users, type User } from '@models/users';
import { APIKeys } from '@models/apikeys';

import {
  fetchUsers,
  createUser as createUserAPI,
  enableUser as enableUserAPI,
  disableUser as disableUserAPI,
} from '@pyroscope/services/users';
import { fetchAPIKeys } from '@pyroscope/services/apiKeys';
import type { RootState } from '../store';
import { addNotification } from './notifications';

interface SettingsRootState {
  // Since the value populated from the server
  // There's no 'loading'
  users:
    | { type: 'loaded'; data?: Users }
    | { type: 'reloading'; data?: Users }
    | { type: 'failed'; data?: Users };

  apiKeys:
    | { type: 'loaded'; data?: APIKeys }
    | { type: 'reloading'; data?: APIKeys }
    | { type: 'failed'; data?: APIKeys };
}

const usersInitialState = { type: 'loaded', data: undefined };
type usersState = typeof usersInitialState;
const apiKeysInitialState = { type: 'loaded', data: undefined };
type apiKeysState = typeof apiKeysInitialState;

// Define the initial state using that type
const initialState: SettingsRootState = {
  users: usersInitialState,
  apiKeys: apiKeysInitialState,
};

export const reloadApiKeys = createAsyncThunk(
  'newRoot/reloadAPIKeys',
  async (foo, thunkAPI) => {
    const res = await fetchAPIKeys();
    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load api keys',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const reloadUsers = createAsyncThunk(
  'newRoot/reloadUsers',
  async (foo, thunkAPI) => {
    const res = await fetchUsers();

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load users',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const enableUser = createAsyncThunk(
  'newRoot/enableUser',
  async (user: User, thunkAPI) => {
    const res = await enableUserAPI(user);

    if (res.isOk) {
      thunkAPI.dispatch(reloadUsers());
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to enable a user',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const disableUser = createAsyncThunk(
  'newRoot/disableUser',
  async (user: User, thunkAPI) => {
    const res = await disableUserAPI(user);

    if (res.isOk) {
      thunkAPI.dispatch(reloadUsers());
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to enable a user',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

// That's only for debugging purposes ATM
export const createUser = createAsyncThunk(
  'newRoot/createUser',
  async (user: User, thunkAPI) => {
    const res = await createUserAPI(user);

    thunkAPI.dispatch(reloadUsers());

    if (res.isOk) {
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to create new user',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const usersSlice = createSlice({
  name: 'users',
  initialState: usersInitialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadUsers.fulfilled, (state, action) => {
      return { type: 'loaded', data: action.payload };
    });
    builder.addCase(reloadUsers.pending, (state) => {
      state = { type: 'reloading', data: state.data };
    });
    builder.addCase(reloadUsers.rejected, (state) => {
      state = { type: 'failed', data: state.data };
    });
  },
});

export const apiKeysSlice = createSlice({
  name: 'apiKeys',
  initialState: apiKeysInitialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadApiKeys.fulfilled, (state, action) => {
      return { type: 'loaded', data: action.payload };
    });
    builder.addCase(reloadApiKeys.pending, (state) => {
      state = { type: 'reloading', data: state.data };
    });
    builder.addCase(reloadUsers.rejected, (state) => {
      state = { type: 'failed', data: state.data };
    });
  },
});

export const settingsState = (state: RootState) => state.settings;

export const usersState = (state: RootState) => state.settings.users;
export const selectUsers = (state: RootState) => state.settings.users.data;

export const apiKeysState = (state: RootState) => state.settings.apiKeys;
export const selectAPIKeys = (state: RootState) => state.settings.apiKeys.data;

export default combineReducers({
  users: usersSlice.reducer,
  apiKeys: apiKeysSlice.reducer,
});
