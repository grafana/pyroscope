import { Profile } from '@pyroscope/models';
import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import { AppNames } from '@webapp/models/appNames';
import { fetchAppNames } from '@webapp/services/appNames';
import { appNameToQuery, queryToAppName } from '@webapp/util/query';
import {
  renderSingle,
  RenderOutput,
  renderDiff,
  RenderDiffResponse,
} from '@webapp/services/render';
import { Timeline } from '@webapp/models/timeline';
import * as tagsService from '@webapp/services/tags';
import type { RootState } from '../store';
import { addNotification } from './notifications';

type SingleView =
  | { type: 'pristine'; profile?: Profile }
  | { type: 'loading'; profile?: Profile }
  | { type: 'loaded'; timeline: Timeline; profile: Profile }
  | { type: 'reloading'; timeline: Timeline; profile: Profile };

type ComparisonView = {
  left:
    | { type: 'pristine'; profile?: Profile }
    | { type: 'loading'; profile?: Profile }
    | { type: 'loaded'; profile: Profile }
    | { type: 'reloading'; profile: Profile };

  right:
    | { type: 'pristine'; profile?: Profile }
    | { type: 'loading'; profile?: Profile }
    | { type: 'loaded'; profile: Profile }
    | { type: 'reloading'; profile: Profile };
};

type TimelineState =
  | { type: 'pristine'; timeline: Timeline }
  | { type: 'loading'; timeline: Timeline }
  | { type: 'loaded'; timeline: Timeline }
  | { type: 'failed'; timeline: Timeline };

type DiffView =
  | { type: 'pristine'; profile?: Profile }
  | { type: 'loading'; profile?: Profile }
  | { type: 'loaded'; profile: Profile }
  | { type: 'reloading'; profile: Profile };

type DiffView2 = ComparisonView;

type TagsData =
  | { type: 'pristine' }
  | { type: 'loading' }
  | { type: 'failed' }
  | { type: 'loaded'; values: string[] };

// Tags really refer to each application
// Should we nest them to an application?
export type TagsState =
  | { type: 'pristine'; tags: Record<string, TagsData> }
  | { type: 'loading'; tags: Record<string, TagsData> }
  | {
      type: 'loaded';
      tags: Record<string, TagsData>;
    }
  | { type: 'failed'; tags: Record<string, TagsData> };

// TODO
type appName = string;
type Tags = Record<appName, TagsState>;

interface ContinuousState {
  from: string;
  until: string;
  leftFrom: string;
  leftUntil: string;
  rightFrom: string;
  rightUntil: string;
  query: string;
  leftQuery?: string;
  rightQuery?: string;
  maxNodes: string;
  refreshToken?: string;

  singleView: SingleView;
  diffView: DiffView;
  diffView2: DiffView2;
  comparisonView: ComparisonView;
  tags: Tags;

  appNames:
    | { type: 'loaded'; data: AppNames }
    | { type: 'reloading'; data: AppNames }
    | { type: 'failed'; data: AppNames };

  // Since both comparison and diff use the same timeline
  // Makes sense storing them separately
  leftTimeline: TimelineState;
  rightTimeline: TimelineState;
}

const initialState: ContinuousState = {
  from: 'now-1h',
  until: 'now',
  leftFrom: 'now-1h',
  leftUntil: 'now-30m',
  rightFrom: 'now-30m',
  rightUntil: 'now',
  maxNodes: '1024',

  singleView: { type: 'pristine' },
  diffView: { type: 'pristine' },
  comparisonView: {
    left: { type: 'pristine' },
    right: { type: 'pristine' },
  },
  diffView2: {
    left: { type: 'pristine' },
    right: { type: 'pristine' },
  },
  tags: {},
  appNames: {
    type: 'loaded',
    data: (window as ShamefulAny).initialState.appNames,
  },
  query: appNameToQuery((window as ShamefulAny).initialState.appNames[0]) ?? '',

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
export const fetchSingleView = createAsyncThunk<
  RenderOutput,
  null,
  { state: { continuous: ContinuousState } }
>('continuous/singleView', async (_, thunkAPI) => {
  const state = thunkAPI.getState();
  const res = await renderSingle(state.continuous);

  if (res.isOk) {
    return Promise.resolve(res.value);
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

export const fetchSideTimelines = createAsyncThunk<
  { left: Timeline; right: Timeline },
  null,
  { state: { continuous: ContinuousState } }
>('continuous/fetchSideTimelines', async (_, thunkAPI) => {
  const state = thunkAPI.getState();

  const res = await Promise.all([
    await renderSingle({
      query: state.continuous.leftQuery || '',
      from: state.continuous.from,
      until: state.continuous.until,
      maxNodes: state.continuous.maxNodes,
      refreshToken: state.continuous.refreshToken,
    }),
    await renderSingle({
      query: state.continuous.rightQuery || '',
      from: state.continuous.from,
      until: state.continuous.until,
      maxNodes: state.continuous.maxNodes,
      refreshToken: state.continuous.refreshToken,
    }),
  ]);

  if (res[0].isOk && res[1].isOk) {
    return Promise.resolve({
      left: res[0].value.timeline,
      right: res[1].value.timeline,
    });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: `Failed to load the timelines`,
      message: '',
      additionalInfo: [res[0].error.message, res[1].error.message],
    })
  );

  return Promise.reject(res.filter((a) => a.isErr).map((a) => a.error));
});

export const fetchComparisonSide = createAsyncThunk<
  { side: 'left' | 'right'; data: Pick<RenderOutput, 'profile'> },
  { side: 'left' | 'right'; query: string },
  { state: { continuous: ContinuousState } }
>('continuous/fetchComparisonSide', async ({ side, query }, thunkAPI) => {
  const state = thunkAPI.getState();

  const res = await (() => {
    switch (side) {
      case 'left': {
        return renderSingle({
          ...state.continuous,
          query,

          from: state.continuous.leftFrom,
          until: state.continuous.leftUntil,
        });
      }
      case 'right': {
        return renderSingle({
          ...state.continuous,
          query,

          from: state.continuous.rightFrom,
          until: state.continuous.rightUntil,
        });
      }
      default: {
        throw new Error('invalid side');
      }
    }
  })();

  if (res.isOk) {
    return Promise.resolve({
      side,
      data: {
        profile: res.value.profile,
      },
    });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: `Failed to load the ${side} side comparison`,
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const fetchDiffView = createAsyncThunk<
  { profile: RenderDiffResponse },
  {
    leftQuery: string;
    leftFrom: string;
    leftUntil: string;
    rightQuery: string;
    rightFrom: string;
    rightUntil: string;
  },
  { state: { continuous: ContinuousState } }
>('continuous/diffView', async (params, thunkAPI) => {
  const state = thunkAPI.getState();
  const res = await renderDiff({
    ...params,
    maxNodes: state.continuous.maxNodes,
  });

  if (res.isOk) {
    return Promise.resolve({ profile: res.value });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load diff view',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const fetchTags = createAsyncThunk(
  'continuous/fetchTags',
  async (query: ContinuousState['query'], thunkAPI) => {
    const appName = queryToAppName(query);
    if (appName.isNothing) {
      return Promise.reject(
        new Error(
          `Query '${appName}' is not a valid app, and it can't have any tags`
        )
      );
    }

    const res = await tagsService.fetchTags(query);

    if (res.isOk) {
      return Promise.resolve({
        appName: appName.value,
        tags: res.value,
      });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load tags',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const fetchTagValues = createAsyncThunk(
  'continuous/fetchTagsValues',
  async (
    payload: { query: ContinuousState['query']; label: string },
    thunkAPI
  ) => {
    const appName = queryToAppName(payload.query);
    if (appName.isNothing) {
      return Promise.reject(
        new Error(
          `Query '${appName}' is not a valid app, and it can't have any tags`
        )
      );
    }

    const res = await tagsService.fetchLabelValues(
      payload.label,
      payload.query
    );

    if (res.isOk) {
      return Promise.resolve({
        appName: appName.value,
        label: payload.label,
        values: res.value,
      });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load tag values',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);

export const reloadAppNames = createAsyncThunk(
  'names/reloadAppNames',
  async (_, thunkAPI) => {
    // TODO, retries?
    const res = await fetchAppNames();

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
  }
);

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
    setQuery(state, action: PayloadAction<string>) {
      // if there's nothing set, pick the first one
      // this likely happened due to the user visiting the root url
      if (!action.payload) {
        const first = state.appNames.data[0];
        if (first) {
          state.query = appNameToQuery(first);
          return;
        }

        // There's not a first one, so leave it it empty
        state.query = '';
        return;
      }
      state.query = action.payload;
    },
    setLeftQuery(state, action: PayloadAction<string>) {
      state.leftQuery = action.payload;
    },
    setRightQuery(state, action: PayloadAction<string>) {
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
    /*************************/
    /*      Single View      */
    /*************************/
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
        ...action.payload,
        type: 'loaded',
      };
    });

    builder.addCase(fetchSingleView.rejected, (state) => {
      switch (state.singleView.type) {
        // if previous state is loaded, let's continue displaying data
        case 'reloading': {
          state.singleView = {
            ...state.singleView,
            type: 'loaded',
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

    /*****************************/
    /*      Comparison View      */
    /*****************************/
    builder.addCase(fetchComparisonSide.pending, (state, action) => {
      const s = state.comparisonView[action.meta.arg.side];
      switch (s.type) {
        case 'loaded':
        case 'reloading': {
          state.comparisonView[action.meta.arg.side] = {
            ...s,
            type: 'reloading',
          };
          break;
        }

        default: {
          state.comparisonView[action.meta.arg.side] = {
            ...state.comparisonView[action.meta.arg.side],
            type: 'loading',
          };
        }
      }
    });

    builder.addCase(fetchComparisonSide.fulfilled, (state, action) => {
      state.comparisonView[action.meta.arg.side] = {
        ...action.payload.data,
        type: 'loaded',
      };
    });

    /*****************************/
    /*      Timeline Sides       */
    /*****************************/
    builder.addCase(fetchSideTimelines.pending, (state) => {
      state.leftTimeline = { ...state.leftTimeline, type: 'loading' };
      state.rightTimeline = { ...state.rightTimeline, type: 'loading' };
    });
    builder.addCase(fetchSideTimelines.fulfilled, (state, action) => {
      state.leftTimeline = {
        type: 'loaded',
        timeline: action.payload.left,
      };
      state.rightTimeline = {
        type: 'loaded',
        timeline: action.payload.right,
      };
    });
    builder.addCase(fetchSideTimelines.rejected, (state) => {
      state.leftTimeline = { ...state.leftTimeline, type: 'failed' };
      state.rightTimeline = { ...state.rightTimeline, type: 'failed' };
    });

    /***********************/
    /*      Diff View      */
    /***********************/
    builder.addCase(fetchDiffView.pending, (state) => {
      switch (state.diffView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
        case 'reloading':
        case 'loaded': {
          state.diffView = {
            ...state.diffView,
            type: 'reloading',
          };
          break;
        }

        default: {
          state.diffView = {
            type: 'loading',
          };
        }
      }
    });

    builder.addCase(fetchDiffView.fulfilled, (state, action) => {
      state.diffView = {
        ...action.payload,
        profile: action.payload.profile,
        type: 'loaded',
      };
    });

    // TODO:
    builder.addCase(fetchTags.pending, (state) => {});

    builder.addCase(fetchTags.fulfilled, (state, action) => {
      // convert each
      const tags = action.payload.tags.reduce((acc, tag) => {
        acc[tag] = { type: 'pristine' };
        return acc;
      }, {} as TagsState['tags']);

      state.tags[action.payload.appName] = {
        type: 'loaded',
        tags,
      };
    });

    // TODO
    builder.addCase(fetchTags.rejected, (state) => {});

    // TODO other cases
    builder.addCase(fetchTagValues.fulfilled, (state, action) => {
      state.tags[action.payload.appName].tags[action.payload.label] = {
        type: 'loaded',
        values: action.payload.values,
      };
    });

    /***********************/
    /*      App Names      */
    /***********************/
    builder.addCase(reloadAppNames.fulfilled, (state, action) => {
      state.appNames = { type: 'loaded', data: action.payload };
    });
    builder.addCase(reloadAppNames.pending, (state) => {
      state.appNames = { type: 'reloading', data: state.appNames.data };
    });
    builder.addCase(reloadAppNames.rejected, (state) => {
      state.appNames = { type: 'failed', data: state.appNames.data };
    });
  },
});

export const selectContinuousState = (state: RootState) => state.continuous;
export default continuousSlice.reducer;
export const { actions } = continuousSlice;
export const { setDateRange, setQuery } = continuousSlice.actions;
export const selectApplicationName = (state: RootState) => {
  return state.continuous.query.split('{')[0];
};

export const selectAppNamesState = (state: RootState) =>
  state.continuous.appNames;
export const selectAppNames = (state: RootState) =>
  state.continuous.appNames.data;

export const selectComparisonState = (state: RootState) =>
  state.continuous.comparisonView;

export const selectIsLoadingData = (state: RootState) => {
  const loadingStates = ['loading', 'reloading'];

  // TODO: should we check if timelines are being reloaded too?
  return (
    loadingStates.includes(state.continuous.singleView.type) ||
    // Comparison
    loadingStates.includes(state.continuous.comparisonView.left.type) ||
    loadingStates.includes(state.continuous.comparisonView.right.type) ||
    // Diff
    loadingStates.includes(state.continuous.diffView.type) ||
    // Timeline Sides
    loadingStates.includes(state.continuous.leftTimeline.type) ||
    loadingStates.includes(state.continuous.rightTimeline.type)
  );
};

export const selectAppTags = (query?: string) => (state: RootState) => {
  if (query) {
    const appName = queryToAppName(query);
    if (appName.isJust) {
      if (state.continuous.tags[appName.value]) {
        return state.continuous.tags[appName.value];
      }
    }
  }

  return {
    type: 'pristine',
    tags: {},
  } as TagsState;
};

export const selectTimelineSidesData = (state: RootState) => {
  return {
    left: state.continuous.leftTimeline.timeline,
    right: state.continuous.rightTimeline.timeline,
  };
};
