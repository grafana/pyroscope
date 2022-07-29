import { Profile } from '@pyroscope/models/src';
import { createAsyncThunk, createSlice, PayloadAction } from '@reduxjs/toolkit';
import { upload } from '@webapp/services/adhoc';
import type { RootState } from '@webapp/redux/store';
import { addNotification } from './notifications';

type SingleView =
  | { type: 'pristine' }
  | { type: 'loading'; fileName: string }
  | { type: 'loaded'; fileName: string; profile: Profile }
  | { type: 'reloading'; fileName: string; profile: Profile };

interface AdhocState {
  singleView: SingleView;
}

const initialState: AdhocState = {
  singleView: { type: 'pristine' },
};

// TODO(eh-am): ask for which view/side it is
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

    // Since the file is invalid, let's remove it
    thunkAPI.dispatch(removeFile('singleView'));

    return Promise.reject(res.error);
  }
);

export const adhocSlice = createSlice({
  name: 'adhoc',
  initialState,
  reducers: {
    removeFile(state, action: PayloadAction<'singleView'>) {
      state[action.payload] = { type: 'pristine' };
    },
  },
  extraReducers: (builder) => {
    builder.addCase(uploadFile.pending, (state, action) => {
      switch (state['singleView'].type) {
        // We already have data
        case 'loaded': {
          state['singleView'] = {
            type: 'reloading',
            fileName: action.meta.arg.name,
            profile: state['singleView'].profile,
          };
          break;
        }

        default: {
          state['singleView'] = {
            type: 'loading',
            fileName: action.meta.arg.name,
          };
        }
      }
    });

    builder.addCase(uploadFile.fulfilled, (state, action) => {
      // It's technically possible to transition rom a non-loading state into a loaded state
      const filename =
        'fileName' in state['singleView'] ? state['singleView'].fileName : '';

      state['singleView'] = {
        type: 'loaded',
        profile: action.payload,
        fileName: filename,
      };
    });
  },
});

export const selectAdhocUpload = (s: 'singleView') => (state: RootState) =>
  state.adhoc[s];

export const selectAdhocUploadedFilename =
  (s: 'singleView') => (state: RootState) => {
    const view = state.adhoc[s];

    if ('fileName' in view) {
      return view.fileName;
    }

    return undefined;
  };

export const { removeFile } = adhocSlice.actions;
export default adhocSlice.reducer;
