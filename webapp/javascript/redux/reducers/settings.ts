import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import { Users } from '@models/users';
import { fetchUsers } from '@pyroscope/services/users';
import type { RootState } from '../store';
import { addNotification } from './notifications';

interface SettingsRootState {
  // Since the value populated from the server
  // There's no 'loading'
  users:
    | { type: 'loaded'; data: Users }
    | { type: 'reloading'; data: Users }
    | { type: 'failed'; data: Users };
}

// Define the initial state using that type
const initialState: SettingsRootState = {
  users: { type: 'loaded', data: (window as any).initialState.users },
};

export const reloadUsers = createAsyncThunk(
  'newRoot/reloadUsers',
  async (foo, thunkAPI) => {
    const res = await fetchUsers();
    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    console.error(res.error.message);

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load app names',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const usersSlice = createSlice({
  name: 'users',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder.addCase(reloadUsers.fulfilled, (state, action) => {
      state.users = { type: 'loaded', data: action.payload };
    });
    builder.addCase(reloadUsers.pending, (state) => {
      state.users = { type: 'reloading', data: state.users.data };
    });
    builder.addCase(reloadUsers.rejected, (state) => {
      state.users = { type: 'failed', data: state.users.data };
    });
  },
});

export const usersState = (state: RootState) => state.settings.users;
export const selectUsers = (state: RootState) => state.settings.users.data;

export default usersSlice.reducer;
