import { Profile } from '@pyroscope/models/src';
import { createAsyncThunk, createSlice, PayloadAction } from '@reduxjs/toolkit';
import { upload, retrieve } from '@webapp/services/adhoc';
import type { RootState } from '@webapp/redux/store';
import { addNotification } from './notifications';

type SingleView =
  | { type: 'pristine' }
  | { type: 'loading'; fileName: string }
  | { type: 'loaded'; fileName: string; profile: Profile }
  | { type: 'reloading'; fileName: string; profile: Profile };

type ComparisonView = {
  left:
    | { type: 'pristine' }
    | { type: 'loading'; fileName: string }
    | { type: 'loaded'; fileName: string; profile: Profile }
    | { type: 'reloading'; fileName: string; profile: Profile };

  right:
    | { type: 'pristine' }
    | { type: 'loading'; fileName: string }
    | { type: 'loaded'; fileName: string; profile: Profile }
    | { type: 'reloading'; fileName: string; profile: Profile };
};

type Shared = {
  left:
    | { type: 'pristine' }
    | { type: 'loading' }
    | { type: 'loaded'; profile: Profile; id: string };

  right:
    | { type: 'pristine' }
    | { type: 'loading' }
    | { type: 'loaded'; profile: Profile; id: string };
};

// The same logic should apply to all sides, the only difference is the data access
type profileSideArgs =
  | { view: 'singleView' }
  | { view: 'comparisonView'; side: 'left' | 'right' };

type profileSideArgs2 =
  | { view: 'singleView'; side: 'left' }
  | { view: 'comparisonView'; side: 'left' | 'right' };

interface AdhocState {
  singleView: SingleView;
  comparisonView: ComparisonView;

  // Shared refers to the list of already uploaded files
  shared: Shared;
}

const initialState: AdhocState = {
  singleView: { type: 'pristine' },
  comparisonView: { left: { type: 'pristine' }, right: { type: 'pristine' } },
  shared: {
    left: { type: 'pristine' },
    right: { type: 'pristine' },
  },
};

export const uploadFile = createAsyncThunk(
  'adhoc/uploadFile',
  async ({ file, ...args }: { file: File } & profileSideArgs2, thunkAPI) => {
    const res = await upload(file);

    if (res.isOk) {
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

export const fetchProfile = createAsyncThunk(
  'adhoc/fetchProfile',
  async ({ id, side }: { id: string; side: 'left' | 'right' }, thunkAPI) => {
    const res = await retrieve(id);

    if (res.isOk) {
      return Promise.resolve({ profile: res.value, side });
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

export const adhocSlice = createSlice({
  name: 'adhoc',
  initialState,
  reducers: {
    removeFile(state, action: PayloadAction<profileSideArgs>) {
      if (action.payload.view === 'comparisonView') {
        state[action.payload.view][action.payload.side] = { type: 'pristine' };
      } else {
        state[action.payload.view] = { type: 'pristine' };
      }
    },
  },
  extraReducers: (builder) => {
    builder.addCase(uploadFile.pending, (state, action) => {
      const s = action.meta.arg;
      const view = (() => {
        if (s.view === 'comparisonView') {
          const view = state[s.view];
          return view[s.side];
        }

        return state[s.view];
      })();

      // TODO(eh-am): clean this all up
      switch (view.type) {
        // We already have data
        case 'loaded': {
          if (s.view === 'comparisonView') {
            state[s.view][s.side] = {
              type: 'reloading',
              fileName: action.meta.arg.file.name,
              profile: view.profile,
            };
          } else {
            state[s.view] = {
              type: 'reloading',
              fileName: action.meta.arg.file.name,
              profile: view.profile,
            };
          }
          break;
        }

        default: {
          if (s.view === 'comparisonView') {
            state[s.view][s.side] = {
              type: 'loading',
              fileName: action.meta.arg.file.name,
            };
          } else {
            state[s.view] = {
              type: 'loading',
              fileName: action.meta.arg.file.name,
            };
          }
        }
      }
    });

    builder.addCase(uploadFile.fulfilled, (state, action) => {
      const s = action.meta.arg;

      state.shared[s.side] = {
        type: 'loaded',
        profile: action.payload.profile,
        id: 'TODO',
      };

      //      if (s.view === 'comparisonView') {
      //        state[s.view][s.side] = {
      //          type: 'loaded',
      //          profile: action.payload.profile,
      //          fileName: action.payload.fileName,
      //        };
      //      } else {
      //        // TODO(eh-am): add filename
      //        state[s.view] = {
      //          type: 'loaded',
      //          profile: action.payload.profile,
      //          fileName: action.payload.fileName,
      //        };
      //      }
    });

    builder.addCase(fetchProfile.fulfilled, (state, action) => {
      const { side } = action.meta.arg;

      state.shared[side] = { type: 'loaded', profile: action.payload.profile };
    });
  },
});

// TODO(eh-am): cleanup view
export const selectAdhocUpload = (s: profileSideArgs) => (state: RootState) => {
  const view = (() => {
    if (s.view === 'comparisonView') {
      const view = state.adhoc[s.view];
      return view[s.side];
    }

    return state.adhoc[s.view];
  })();

  return view;
};

export const selectAdhocUploadedFilename =
  (s: profileSideArgs) => (state: RootState) => {
    const view = (() => {
      if (s.view === 'comparisonView') {
        const view = state.adhoc[s.view];
        return view[s.side];
      }

      return state.adhoc[s.view];
    })();

    if ('fileName' in view) {
      return view.fileName;
    }

    return undefined;
  };

export const selectShared = (state: RootState) => {
  return state.adhoc.shared;
};

export const { removeFile } = adhocSlice.actions;
export default adhocSlice.reducer;
