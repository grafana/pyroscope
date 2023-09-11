import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import type { Profile } from '@pyroscope/legacy/models';
import {
  MergeOutput,
  mergeWithQueryID,
  HeatmapOutput,
  getHeatmap,
  SelectionProfileOutput,
  getHeatmapSelectionProfile,
  Heatmap,
  GetHeatmapProps,
  SelectionProfileProps,
} from '@pyroscope/services/render';
import type { RootState } from '@pyroscope/redux/store';
import { RequestAbortedError } from '@pyroscope/services/base';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import { createAsyncThunk } from '../async-thunk';

type MergeMetadata = {
  appName: string;
  startTime: string;
  endTime: string;
  profilesLength: number;
};

type SingleView =
  | { type: 'pristine'; profile?: Profile; mergeMetadata?: MergeMetadata }
  | { type: 'loading'; profile?: Profile; mergeMetadata?: MergeMetadata }
  | {
      type: 'loaded';
      profile: Profile;
      mergeMetadata: MergeMetadata;
    }
  | {
      type: 'reloading';
      profile: Profile;
      mergeMetadata: MergeMetadata;
    };
// TODO

type ExemplarsSingleView =
  | {
      type: 'pristine';
      heatmap?: Heatmap | null;
      profile?: Profile;
      selectionProfile?: Profile;
    }
  | {
      type: 'loading';
      heatmap?: Heatmap | null;
      profile?: Profile;
      selectionProfile?: Profile;
    }
  | {
      type: 'loaded';
      heatmap: Heatmap | null;
      profile?: Profile;
      selectionProfile?: Profile;
    }
  | {
      type: 'reloading';
      heatmap: Heatmap | null;
      profile?: Profile;
      selectionProfile?: Profile;
    };

interface TracingState {
  queryID: string;
  maxNodes: string;
  refreshToken?: string;

  exemplarsSingleView: ExemplarsSingleView;
  singleView: SingleView;
}

let singleViewAbortController: AbortController | undefined;
let exemplarsSingleViewAbortController: AbortController | undefined;
let selectionProfileAbortController: AbortController | undefined;

const initialState: TracingState = {
  queryID: '',
  maxNodes: '1024',

  exemplarsSingleView: { type: 'pristine' },
  singleView: { type: 'pristine' },
};

export const fetchSingleView = createAsyncThunk<
  MergeOutput,
  null,
  { state: { tracing: TracingState } }
>('tracing/singleView', async (_, thunkAPI) => {
  if (singleViewAbortController) {
    singleViewAbortController.abort();
  }

  singleViewAbortController = new AbortController();
  thunkAPI.signal = singleViewAbortController.signal;

  const state = thunkAPI.getState();
  const res = await mergeWithQueryID(state.tracing, singleViewAbortController);

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  if (res.isErr && res.error instanceof RequestAbortedError) {
    return thunkAPI.rejectWithValue({ rejectedWithValue: 'reloading' });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load single view data',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const fetchExemplarsSingleView = createAsyncThunk<
  HeatmapOutput,
  GetHeatmapProps,
  { state: { tracing: TracingState } }
>('tracing/exemplarsSingleView', async (heatmapProps, thunkAPI) => {
  if (exemplarsSingleViewAbortController) {
    exemplarsSingleViewAbortController.abort();
  }

  exemplarsSingleViewAbortController = new AbortController();
  thunkAPI.signal = exemplarsSingleViewAbortController.signal;

  const res = await getHeatmap(
    heatmapProps,
    exemplarsSingleViewAbortController
  );

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  if (res.isErr && res.error instanceof RequestAbortedError) {
    return thunkAPI.rejectWithValue({ rejectedWithValue: 'reloading' });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load heatmap',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const fetchSelectionProfile = createAsyncThunk<
  SelectionProfileOutput,
  SelectionProfileProps,
  { state: { tracing: TracingState } }
>('tracing/fetchSelectionProfile', async (selectionProfileProps, thunkAPI) => {
  if (selectionProfileAbortController) {
    selectionProfileAbortController.abort();
  }

  selectionProfileAbortController = new AbortController();
  thunkAPI.signal = selectionProfileAbortController.signal;

  const res = await getHeatmapSelectionProfile(
    selectionProfileProps,
    selectionProfileAbortController
  );

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  if (res.isErr && res.error instanceof RequestAbortedError) {
    return thunkAPI.rejectWithValue({ rejectedWithValue: 'reloading' });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load profile',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const tracingSlice = createSlice({
  name: 'tracing',
  initialState,
  reducers: {
    setMaxNodes(state, action: PayloadAction<string>) {
      state.maxNodes = action.payload;
    },
    setQueryID(state, action: PayloadAction<string>) {
      state.queryID = action.payload;
    },
    refresh(state) {
      state.refreshToken = Math.random().toString();
    },
  },
  extraReducers: (builder) => {
    /** ********************** */
    /*      Single View      */
    /** ********************** */
    builder.addCase(fetchSingleView.pending, (state) => {
      switch (state.singleView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
        case 'reloading':
        case 'loaded': {
          state.singleView = {
            ...state.singleView,
            type: 'reloading',
          };
          break;
        }

        default: {
          state.singleView = { type: 'loading' };
        }
      }
    });

    builder.addCase(fetchSingleView.fulfilled, (state, action) => {
      state.singleView = {
        ...action.payload,
        mergeMetadata: action.payload.mergeMetadata,
        type: 'loaded',
      };
    });

    builder.addCase(fetchSingleView.rejected, (state, action) => {
      switch (state.singleView.type) {
        // if previous state is loaded, let's continue displaying data
        case 'reloading': {
          let type: SingleView['type'] = 'reloading';
          if (action.meta.rejectedWithValue) {
            type = (
              action?.payload as { rejectedWithValue: SingleView['type'] }
            )?.rejectedWithValue;
          } else if (action.error.message === 'cancel') {
            type = 'loaded';
          }
          state.singleView = {
            ...state.singleView,
            type,
          };
          break;
        }

        default: {
          // it failed to load for the first time, so far all effects it's pristine
          state.singleView = {
            type: 'pristine',
          };
        }
      }
    });

    /** ******************************** */
    /*      Exemplars Single View      */
    /** ******************************** */

    builder.addCase(fetchExemplarsSingleView.pending, (state) => {
      switch (state.exemplarsSingleView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
        case 'reloading':
        case 'loaded': {
          state.exemplarsSingleView = {
            ...state.exemplarsSingleView,
            type: 'reloading',
          };
          break;
        }

        default: {
          state.exemplarsSingleView = { type: 'loading' };
        }
      }
    });

    builder.addCase(fetchExemplarsSingleView.fulfilled, (state, action) => {
      state.exemplarsSingleView = {
        ...action.payload,
        type: 'loaded',
      };
    });

    builder.addCase(fetchExemplarsSingleView.rejected, (state, action) => {
      switch (state.exemplarsSingleView.type) {
        // if previous state is loaded, let's continue displaying data
        case 'reloading': {
          let type: ExemplarsSingleView['type'] = 'reloading';
          if (action.meta.rejectedWithValue) {
            type = (
              action?.payload as {
                rejectedWithValue: ExemplarsSingleView['type'];
              }
            )?.rejectedWithValue;
          } else if (action.error.message === 'cancel') {
            type = 'loaded';
          }
          state.exemplarsSingleView = {
            ...state.exemplarsSingleView,
            type,
          };
          break;
        }

        default: {
          // it failed to load for the first time, so far all effects it's pristine
          state.exemplarsSingleView = {
            type: 'pristine',
          };
        }
      }
    });

    /** *********************************** */
    /*      Heatmap Selection Profile      */
    /** *********************************** */

    builder.addCase(fetchSelectionProfile.pending, (state) => {
      switch (state.exemplarsSingleView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
        case 'reloading':
        case 'loaded': {
          state.exemplarsSingleView = {
            ...state.exemplarsSingleView,
            type: 'reloading',
          };
          break;
        }

        default: {
          state.exemplarsSingleView = { type: 'loading' };
        }
      }
    });

    builder.addCase(fetchSelectionProfile.fulfilled, (state, action) => {
      state.exemplarsSingleView.type = 'loaded';
      state.exemplarsSingleView.selectionProfile =
        action.payload.selectionProfile;
    });

    builder.addCase(fetchSelectionProfile.rejected, (state, action) => {
      switch (state.exemplarsSingleView.type) {
        // if previous state is loaded, let's continue displaying data
        case 'reloading': {
          let type: ExemplarsSingleView['type'] = 'reloading';
          if (action.meta.rejectedWithValue) {
            type = (
              action?.payload as {
                rejectedWithValue: ExemplarsSingleView['type'];
              }
            )?.rejectedWithValue;
          } else if (action.error.message === 'cancel') {
            type = 'loaded';
          }
          state.exemplarsSingleView = {
            ...state.exemplarsSingleView,
            type,
          };
          break;
        }

        default: {
          // it failed to load for the first time, so far all effects it's pristine
          state.exemplarsSingleView = {
            type: 'pristine',
          };
        }
      }
    });
  },
});

export const selectTracingState = (state: RootState) => state.tracing;

export default tracingSlice.reducer;
export const { actions } = tracingSlice;
