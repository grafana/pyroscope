import { createSlice, combineReducers } from '@reduxjs/toolkit';
import { Users, type User } from '@pyroscope/models/users';
import { APIKey, APIKeys } from '@pyroscope/models/apikeys';
import { App } from '@pyroscope/models/app';

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
import { fetchApps, deleteApp as deleteAppAPI } from '@pyroscope/services/apps';
import type { RootState } from '@pyroscope/redux/store';
import { addNotification } from './notifications';
import { createAsyncThunk } from '../async-thunk';

enum FetchStatus {
  pristine = 'pristine',
  loading = 'loading',
  loaded = 'loaded',
  failed = 'failed',
}
type DataWithStatus<T> = { type: FetchStatus; data?: T };

const usersInitialState: DataWithStatus<Users> = {
  type: FetchStatus.pristine,
  data: undefined,
};

const apiKeysInitialState: DataWithStatus<APIKeys> = {
  type: FetchStatus.pristine,
  data: undefined,
};

const appsInitialState: DataWithStatus<App[]> = {
  type: FetchStatus.pristine,
  data: undefined,
};

export const reloadApiKeys = createAsyncThunk(
  'newRoot/reloadAPIKeys',
  async (_, thunkAPI) => {
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
  async (_, thunkAPI) => {
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

export const reloadApps = createAsyncThunk(
  'newRoot/reloadApps',
  async (_, thunkAPI) => {
    const res = await fetchApps();

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load apps',
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
        title: 'Failed to disable a user',
        message: res.error.message,
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
        title: 'Failed to create new user',
        message: res.error.message,
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
        title: 'Failed to delete user',
        message: res.error.message,
      })
    );
    return Promise.reject(res.error);
  }
);

export const changeUserRole = createAsyncThunk(
  'users/changeUserRole',
  async (action: Pick<User, 'id' | 'role'>, thunkAPI) => {
    const { id, role } = action;
    const res = await changeUserRoleAPI(id, role);

    if (res.isOk) {
      return Promise.resolve(true);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to change users role',
        message: res.error.message,
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const createAPIKey = createAsyncThunk(
  'newRoot/createAPIKey',
  async (data: Parameters<typeof createAPIKeyAPI>[0], thunkAPI) => {
    const res = await createAPIKeyAPI(data);

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to create API key',
        message: res.error.message,
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const deleteAPIKey = createAsyncThunk(
  'newRoot/deleteAPIKey',
  async (data: Pick<APIKey, 'id'>, thunkAPI) => {
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
        title: 'Failed to delete API key',
        message: res.error.message,
      })
    );
    return thunkAPI.rejectWithValue(res.error);
  }
);

export const deleteApp = createAsyncThunk(
  'newRoot/deleteApp',
  async (app: App, thunkAPI) => {
    const res = await deleteAppAPI({ name: app.name });

    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    thunkAPI.dispatch(reloadApps());

    if (res.isOk) {
      return Promise.resolve(true);
    }

    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to delete app',
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
      return { type: FetchStatus.loaded, data: action.payload };
    });

    builder.addCase(reloadUsers.pending, (state) => {
      return { type: FetchStatus.loading, data: state.data };
    });
    builder.addCase(reloadUsers.rejected, (state) => {
      return { type: FetchStatus.failed, data: state.data };
    });
  },
});

export const apiKeysSlice = createSlice({
  name: 'apiKeys',
  initialState: apiKeysInitialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadApiKeys.fulfilled, (_, action) => {
      return { type: FetchStatus.loaded, data: action.payload };
    });
    builder.addCase(reloadApiKeys.pending, (state) => {
      return { type: FetchStatus.loading, data: state.data };
    });
    builder.addCase(reloadApiKeys.rejected, (state) => {
      return { type: FetchStatus.failed, data: state.data };
    });
  },
});

export const appsSlice = createSlice({
  name: 'apps',
  initialState: appsInitialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadApps.fulfilled, (_, action) => {
      return { type: FetchStatus.loaded, data: action.payload };
    });
    builder.addCase(reloadApps.pending, (state) => {
      return { type: FetchStatus.loading, data: state.data };
    });
    builder.addCase(reloadApps.rejected, (state) => {
      return { type: FetchStatus.failed, data: state.data };
    });
  },
});

export const settingsState = (state: RootState) => state.settings;

export const usersState = (state: RootState) => state.settings.users;
export const selectUsers = (state: RootState) => state.settings.users.data;

export const apiKeysState = (state: RootState) => state.settings.apiKeys;
export const selectAPIKeys = (state: RootState) => state.settings.apiKeys.data;

export const appsState = (state: RootState) => state.settings.apps;
export const selectApps = (state: RootState) => state.settings.apps.data;
export const selectIsLoadingApps = (state: RootState) => {
  return state.settings.apps.type === FetchStatus.loading;
};

export default combineReducers({
  users: usersSlice.reducer,
  apiKeys: apiKeysSlice.reducer,
  apps: appsSlice.reducer,
});
