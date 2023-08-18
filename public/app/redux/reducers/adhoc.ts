import { Profile } from '@pyroscope/legacy/models';
import { createAsyncThunk, createSlice, PayloadAction } from '@reduxjs/toolkit';
import {
  upload,
  retrieve,
  retrieveAll,
  retrieveDiff,
} from '@pyroscope/services/adhoc';
import type { RootState } from '@pyroscope/redux/store';
import { Maybe } from '@pyroscope/util/fp';
import { AllProfiles } from '@pyroscope/models/adhoc';
import { addNotification } from '@pyroscope/redux/reducers/notifications';

type uploadState =
  | { type: 'pristine' }
  | { type: 'loading'; fileName: string }
  | { type: 'loaded' };

type Upload = {
  left: uploadState;
  right: uploadState;
};

type Shared = {
  profilesList:
    | { type: 'pristine' }
    | { type: 'loading' }
    | { type: 'loaded'; profilesList: AllProfiles };

  left: {
    type: 'pristine' | 'loading' | 'loaded';
    profile?: Profile;
    id?: string;
  };

  right: {
    type: 'pristine' | 'loading' | 'loaded';
    profile?: Profile;
    id?: string;
  };
};

type DiffState = {
  type: 'pristine' | 'loading' | 'loaded';
  profile?: Profile;
};

type side = 'left' | 'right';

interface AdhocState {
  // Upload refers to the files being uploaded
  upload: Upload;
  // Shared refers to the list of already uploaded files
  shared: Shared;
  diff: DiffState;
}

const initialState: AdhocState = {
  shared: {
    profilesList: { type: 'pristine' },
    left: { type: 'pristine' },
    right: { type: 'pristine' },
  },
  upload: { left: { type: 'pristine' }, right: { type: 'pristine' } },
  diff: { type: 'pristine' },
};

export const uploadFile = createAsyncThunk(
  'adhoc/uploadFile',
  async (
    {
      file,
      spyName,
      units,
      ...args
    }: { file: File; spyName?: string; units?: string } & { side: side },
    thunkAPI
  ) => {
    const res = await upload(
      file,
      spyName && units ? { spyName, units } : undefined
    );

    if (res.isOk) {
      // Since we just uploaded a file, let's reload to see it on the file list
      thunkAPI.dispatch(fetchAllProfiles());

      return Promise.resolve({ profile: res.value, fileName: file.name });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to upload adhoc file',
        message: res.error.message,
      })
    );

    // Since the file is invalid, let's remove it
    thunkAPI.dispatch(removeFile(args));

    return Promise.reject(res.error);
  }
);

export const fetchAllProfiles = createAsyncThunk(
  'adhoc/fetchAllProfiles',
  async (_, thunkAPI) => {
    const res = await retrieveAll();
    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load list of adhoc files',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const fetchProfile = createAsyncThunk(
  'adhoc/fetchProfile',
  async ({ id, side }: { id: string; side: side }, thunkAPI) => {
    const res = await retrieve(id);

    if (res.isOk) {
      return Promise.resolve({ profile: res.value, side, id });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load adhoc file',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const fetchDiffProfile = createAsyncThunk(
  'adhoc/fetchDiffProfile',
  async (
    { leftId, rightId }: { leftId: string; rightId: string },
    thunkAPI
  ) => {
    const res = await retrieveDiff(leftId, rightId);

    if (res.isOk) {
      return Promise.resolve({ profile: res.value });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load adhoc diff',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const adhocSlice = createSlice({
  name: 'adhoc',
  initialState,
  reducers: {
    removeFile(state, action: PayloadAction<{ side: side }>) {
      state.upload[action.payload.side] = {
        type: 'pristine',
      };
    },
  },
  extraReducers: (builder) => {
    builder.addCase(uploadFile.pending, (state, action) => {
      state.upload[action.meta.arg.side] = {
        type: 'loading',
        fileName: action.meta.arg.file.name,
      };
    });
    builder.addCase(uploadFile.rejected, (state, action) => {
      // Since the file is invalid, let's remove it
      state.upload[action.meta.arg.side] = {
        type: 'pristine',
      };
    });

    builder.addCase(uploadFile.fulfilled, (state, action) => {
      const s = action.meta.arg;

      //      state.upload[s.side] = { type: 'loaded', fileName: s.file.name };
      state.upload[s.side] = { type: 'pristine' };

      state.shared[s.side] = {
        type: 'loaded',
        profile: action.payload.profile.flamebearer,
        id: action.payload.profile.id,
      };
    });

    builder.addCase(fetchProfile.fulfilled, (state, action) => {
      const { side } = action.meta.arg;

      // After loading a profile, there's no uploaded profile
      state.upload[side] = {
        type: 'pristine',
      };

      state.shared[side] = {
        type: 'loaded',
        profile: action.payload.profile,
        id: action.payload.id,
      };
    });

    builder.addCase(fetchAllProfiles.fulfilled, (state, action) => {
      state.shared.profilesList = {
        type: 'loaded',
        profilesList: action.payload,
      };
    });

    builder.addCase(fetchDiffProfile.pending, (state) => {
      state.diff = {
        // Keep previous value
        ...state.diff,
        type: 'loading',
      };
    });

    builder.addCase(fetchDiffProfile.fulfilled, (state, action) => {
      state.diff = {
        type: 'loaded',
        profile: action.payload.profile,
      };
    });
  },
});

const selectAdhocState = (state: RootState) => {
  return state.adhoc;
};

export const selectShared = (state: RootState) => {
  return selectAdhocState(state).shared;
};

export const selectProfilesList = (state: RootState) => {
  return selectShared(state).profilesList;
};

export const selectedSelectedProfileId = (side: side) => (state: RootState) => {
  return Maybe.of(selectShared(state)[side].id);
};

export const selectProfile = (side: side) => (state: RootState) => {
  return Maybe.of(selectShared(state)[side].profile);
};

export const selectDiffProfile = (state: RootState) => {
  return Maybe.of(selectAdhocState(state).diff.profile);
};

export const selectProfileId = (side: side) => (state: RootState) => {
  return Maybe.of(selectShared(state)[side].id);
};

export const { removeFile } = adhocSlice.actions;
export default adhocSlice.reducer;
