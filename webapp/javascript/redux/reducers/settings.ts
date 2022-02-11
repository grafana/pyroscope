import {
  createSlice,
  createAsyncThunk,
  combineReducers,
} from '@reduxjs/toolkit';
import { Users, type User } from '@models/users';
import { APIKey, APIKeys } from '@models/apikeys';

import {
  fetchUsers,
  createUser as createUserAPI,
  enableUser as enableUserAPI,
  disableUser as disableUserAPI,
  changeUserRole as changeUserRoleAPI,
  deleteUser as deleteUserAPI,
} from '@pyroscope/services/users';
import {
  fetchAPIKeys,
  createAPIKey as createAPIKeyAPI,
  deleteAPIKey as deleteAPIKeyAPI,
} from '@pyroscope/services/apiKeys';
import type { RootState } from '../store';
import { addNotification } from './notifications';

type UsersState = {
  type: 'loaded' | 'reloading' | 'failed';
  data?: Users;
};

type APIKeysState = {
  type: string;
  data?: APIKeys;
};
interface SettingsRootState {
  // Since the value populated from the server
  // There's no 'loading'
  users: UsersState;
  apiKeys: APIKeysState;
}

const usersInitialState = {
  type: 'loaded',
  data: undefined,
};
const apiKeysInitialState = { type: 'loaded', data: undefined };

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
        title: 'Failed',
        message: 'Failed to load users',
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
        title: 'Failed',
        message: 'Failed to enable a user',
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
        title: 'Failed',
        message: 'Failed to disable a user',
      })
    );

    return Promise.reject(res.error);
  }
);

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
        title: 'Failed',
        message: 'Failed to create new user',
      })
    );
    return Promise.reject(res.error);
  }
);

export const deleteUser = createAsyncThunk(
  'newRoot/deleteUser',
  async (user: User, thunkAPI) => {
    const res = await deleteUserAPI({ id: user.id });

    thunkAPI.dispatch(reloadUsers());

    if (res.isOk) {
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed',
        message: 'Failed to delete user',
      })
    );
    return Promise.reject(res.error);
  }
);

export const changeUserRole = createAsyncThunk(
  'users/changeUserRole',
  async (action: Partial<User>, thunkAPI) => {
    const { id, role } = action;
    const res = await changeUserRoleAPI({ id }, role);

    if (res.isOk) {
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed',
        message: 'Failed to change users role',
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const createAPIKey = createAsyncThunk(
  'newRoot/createAPIKey',
  async (data, thunkAPI) => {
    const res = await createAPIKeyAPI(data);

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to create API key',
        message: res.error.errors.join(', '),
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const deleteAPIKey = createAsyncThunk(
  'newRoot/deleteAPIKey',
  async (data: Partial<APIKey>, thunkAPI) => {
    const res = await deleteAPIKeyAPI(data);
    if (res.isOk) {
      thunkAPI.dispatch(
        addNotification({
          type: 'success',
          title: 'Key has been deleted',
          message: `API Key id ${data.id} has been successfully deleted`,
        })
      );
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to create API key',
        message: res.error.errors.join(', '),
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const usersSlice = createSlice({
  name: 'users',
  initialState: usersInitialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadUsers.fulfilled, (_, action) => {
      return { type: 'loaded', data: action.payload };
    });
    builder.addCase(reloadUsers.pending, (state) => {
      return { type: 'reloading', data: state.data };
    });
    builder.addCase(reloadUsers.rejected, (state) => {
      return { type: 'failed', data: state.data };
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
      return { type: 'reloading', data: state.data };
    });
    builder.addCase(reloadUsers.rejected, (state) => {
      return { type: 'failed', data: state.data };
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
