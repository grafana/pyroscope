import { Profile } from '@pyroscope/models';
import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import { renderSingle, RenderOutput } from '../../services/render';
import { addNotification } from './notifications';

type SingleView =
  | { type: 'pristine' }
  | { type: 'loading' }
  | {
      type: 'loaded';
      // TODO: type this
      timeline: any;
      profile: Profile;
    }
  | {
      type: 'reloading';
      // TODO: type this
      timeline: any;
      profile: Profile;
    };

export const fetchSingleView = createAsyncThunk<
  RenderOutput,
  null,
  { state: { continuous: ContinuousState } }
>('continuous/singleView', async (a, thunkAPI) => {
  const state = thunkAPI.getState();
  const res = await renderSingle(state.continuous);

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed',
      message: `Failed to load singleView`,
    })
  );

  return Promise.reject(res.error);
});

interface ContinuousState {
  from: string;
  until: string;
  leftFrom: string;
  leftUntil: string;
  rightFrom: string;
  rightUntil: string;
  query: string;
  maxNodes: string;
  refreshToken?: string;

  singleView: SingleView;
}

const initialState: ContinuousState = {
  from: 'now-1h',
  until: 'now',
  leftFrom: 'now-1h',
  leftUntil: 'now-30m',
  rightFrom: 'now-30m',
  rightUntil: 'now',
  query: '',
  maxNodes: '1024',

  singleView: { type: 'pristine' },
};

export const continuousSlice = createSlice({
  name: 'continuous',
  initialState,
  reducers: {
    setFrom(state, action: PayloadAction<string>) {
      state.from = action.payload;
    },
    setQuery(state, action: PayloadAction<string>) {
      state.query = action.payload;
    },
    setUntil(state, action: PayloadAction<string>) {
      state.until = action.payload;
    },
    setLeftFrom(state, action: PayloadAction<string>) {
      state.leftFrom = action.payload;
    },
    setLeftUntil(state, action: PayloadAction<string>) {
      state.leftUntil = action.payload;
    },
    setRightFrom(state, action: PayloadAction<string>) {
      state.rightFrom = action.payload;
    },
    setRightUntil(state, action: PayloadAction<string>) {
      state.rightUntil = action.payload;
    },
    setMaxNodes(state, action: PayloadAction<string>) {
      state.maxNodes = action.payload;
    },
  },
  extraReducers: (builder) => {
    builder.addCase(fetchSingleView.pending, (state) => {
      switch (state.singleView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
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
        type: 'loaded',
        ...action.payload,
        timeline: null,
      };
    });
  },
});

export default continuousSlice.reducer;
export const { actions } = continuousSlice;
