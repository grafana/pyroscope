import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import { Query } from '@pyroscope/models/query';
import { addNotification } from '../notifications';
import { createAsyncThunk } from '../../async-thunk';
import { ContinuousState, TagsState } from './state';
import { fetchTagValues, fetchTags } from './tags.thunks';
import { fetchSingleView } from './singleView.thunks';
import { fetchSideTimelines } from './timelines.thunks';
import { fetchApps } from '@pyroscope/services/apps';
import { formatAsOBject } from '@pyroscope/util/formatDate';
import { App } from '@pyroscope/models/app';

const initialState: ContinuousState = {
  from: 'now-1h',
  until: 'now',
  leftFrom: 'now-1h',
  leftUntil: 'now-30m',
  rightFrom: 'now-30m',
  rightUntil: 'now',
  maxNodes: '1024',
  aggregation: 'sum',

  singleView: { type: 'pristine' },
  tags: {},

  apps: {
    type: 'loaded',
    data: [],
  },

  query: '',

  leftTimeline: {
    type: 'pristine',
    timeline: {
      startTime: 0,
      samples: [],
      durationDelta: 0,
    },
  },
  rightTimeline: {
    type: 'pristine',
    timeline: {
      startTime: 0,
      samples: [],
      durationDelta: 0,
    },
  },
};

export const reloadAppNames = createAsyncThunk<
  App[],
  void,
  { state: { continuous: ContinuousState } }
>('names/reloadAppNames', async (_, thunkAPI) => {
  const state = thunkAPI.getState();
  const fromMs = formatAsOBject(state.continuous.from).getTime();
  const untilMs = formatAsOBject(state.continuous.until).getTime();

  // TODO, retries?
  const res = await fetchApps(fromMs, untilMs);

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load app names',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const continuousSlice = createSlice({
  name: 'continuous',
  initialState,
  reducers: {
    setFrom(state, action: PayloadAction<string>) {
      state.from = action.payload;
    },
    setUntil(state, action: PayloadAction<string>) {
      state.until = action.payload;
    },
    setFromAndUntil(
      state,
      action: PayloadAction<{ from: string; until: string }>
    ) {
      state.from = action.payload.from;
      state.until = action.payload.until;
    },
    setQuery(state, action: PayloadAction<Query>) {
      // TODO: figure out why is being dispatched as undefined
      state.query = action.payload || '';
    },
    setLeftQuery(state, action: PayloadAction<Query>) {
      state.leftQuery = action.payload;
    },
    setRightQuery(state, action: PayloadAction<Query>) {
      state.rightQuery = action.payload;
    },
    setLeftFrom(state, action: PayloadAction<string>) {
      state.leftFrom = action.payload;
    },
    setLeftUntil(state, action: PayloadAction<string>) {
      state.leftUntil = action.payload;
    },
    setLeft(state, action: PayloadAction<{ from: string; until: string }>) {
      state.leftFrom = action.payload.from;
      state.leftUntil = action.payload.until;
    },
    setRightFrom(state, action: PayloadAction<string>) {
      state.rightFrom = action.payload;
    },
    setRightUntil(state, action: PayloadAction<string>) {
      state.rightUntil = action.payload;
    },
    setRight(state, action: PayloadAction<{ from: string; until: string }>) {
      state.rightFrom = action.payload.from;
      state.rightUntil = action.payload.until;
    },
    setMaxNodes(state, action: PayloadAction<string>) {
      state.maxNodes = action.payload;
    },
    setAggregation(state, action: PayloadAction<string>) {
      state.aggregation = action.payload;
    },

    setDateRange(
      state,
      action: PayloadAction<Pick<ContinuousState, 'from' | 'until'>>
    ) {
      state.from = action.payload.from;
      state.until = action.payload.until;
    },

    refresh(state) {
      state.refreshToken = Math.random().toString();
    },
  },

  extraReducers: (builder) => {
    /** ******************* */
    /* GENERAL GUIDELINES */
    /** ******************* */

    // There are (currently) only 2 ways an action can be aborted:
    // 1. The component is unmounting, eg when changing route
    // 2. New data is loading, which means previous request is going to be superseeded
    // In both cases, not doing state transitions is fine
    // Specially in the second case, where a 'rejected' may happen AFTER a 'pending' is dispatched
    // https://redux-toolkit.js.org/api/createAsyncThunk#checking-if-a-promise-rejection-was-from-an-error-or-cancellation

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
        type: 'loaded',
      };
    });

    builder.addCase(fetchSingleView.rejected, (state, action) => {
      if (action.meta.aborted) {
        return;
      }

      state.singleView = {
        type: 'pristine',
      };
    });

    /** ************************** */
    /*      Timeline Sides       */
    /** ************************** */
    builder.addCase(fetchSideTimelines.pending, (state) => {
      state.leftTimeline = {
        ...state.leftTimeline,
        type: getNextStateFromPending(state.leftTimeline.type),
      };
      state.rightTimeline = {
        ...state.rightTimeline,
        type: getNextStateFromPending(state.leftTimeline.type),
      };
    });
    builder.addCase(fetchSideTimelines.fulfilled, (state, action) => {
      state.leftTimeline = {
        type: 'loaded',
        timeline: action.payload.left.timeline,
      };
      state.rightTimeline = {
        type: 'loaded',
        timeline: action.payload.right.timeline,
      };
    });

    // TODO
    builder.addCase(fetchSideTimelines.rejected, () => {});

    /** ************** */
    /*      Tags     */
    /** ************** */

    // TODO:
    builder.addCase(fetchTags.pending, () => {});

    builder.addCase(fetchTags.fulfilled, (state, action) => {
      // convert each
      // TODO(eh-am): don't delete tags if we already have them
      const tags = action.payload.tags.reduce((acc, tag) => {
        acc[tag] = { type: 'pristine' };
        return acc;
      }, {} as TagsState['tags']);

      state.tags[action.payload.appName] = {
        type: 'loaded',
        from: action.payload.from,
        until: action.payload.until,
        tags,
      };
    });

    // TODO
    builder.addCase(fetchTags.rejected, () => {});

    // TODO other cases
    builder.addCase(fetchTagValues.fulfilled, (state, action) => {
      state.tags[action.payload.appName].tags[action.payload.label] = {
        type: 'loaded',
        values: action.payload.values,
      };
    });

    /** ******************** */
    /*      App Names      */
    /** ******************** */
    builder.addCase(reloadAppNames.fulfilled, (state, action) => {
      state.apps = { type: 'loaded', data: action.payload };
    });
    builder.addCase(reloadAppNames.pending, (state) => {
      state.apps = { type: 'reloading', data: state.apps.data };
    });
    builder.addCase(reloadAppNames.rejected, (state) => {
      state.apps = { type: 'failed', data: state.apps.data };
    });
  },
});

export const { actions } = continuousSlice;
export const { setDateRange, setQuery, setMaxNodes } = continuousSlice.actions;

function getNextStateFromPending(
  prevState: 'pristine' | 'loading' | 'reloading' | 'loaded'
) {
  if (prevState === 'pristine' || prevState === 'loading') {
    return 'loading';
  }

  return 'reloading';
}

export * from './selectors';

export * from './state';
export * from './tags.thunks';
export * from './singleView.thunks';
export * from './timelines.thunks';

export const continuousReducer = continuousSlice.reducer;
