import { Profile } from '@pyroscope/models';
import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import { AppNames } from '@webapp/models/appNames';
import { fetchAppNames } from '@webapp/services/appNames';
import { appNameToQuery, queryToAppName } from '@webapp/util/query';
import {
  renderSingle,
  RenderOutput,
  renderDiff,
} from '@webapp/services/render';
import { Timeline } from '@webapp/models/timeline';
import * as tagsService from '@webapp/services/tags';
import type { RootState } from '../store';
import { addNotification } from './notifications';

type SingleView =
  | { type: 'pristine' }
  | { type: 'loading' }
  | {
      type: 'loaded';
      timeline: Timeline;
      profile: Profile;
    }
  | {
      type: 'reloading';
      timeline: Timeline;
      profile: Profile;
    };

type ComparisonView = {
  timeline:
    | { type: 'pristine' }
    | { type: 'loading' }
    | {
        type: 'loaded';
        data: Timeline;
      }
    | {
        type: 'reloading';
        data: Timeline;
      };

  left:
    | { type: 'pristine' }
    | { type: 'loading' }
    | {
        type: 'loaded';
        timeline: Timeline;
        profile: Profile;
      }
    | {
        type: 'reloading';
        timeline: Timeline;
        profile: Profile;
      };

  right:
    | { type: 'pristine' }
    | { type: 'loading' }
    | {
        type: 'loaded';
        timeline: Timeline;
        profile: Profile;
      }
    | {
        type: 'reloading';
        timeline: Timeline;
        profile: Profile;
      };
};

type DiffView =
  | { type: 'pristine' }
  | { type: 'loading' }
  | {
      type: 'loaded';
      timeline: Timeline;
      profile: Profile;
    }
  | {
      type: 'reloading';
      timeline: Timeline;
      profile: Profile;
    };

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
  comparisonView: ComparisonView;
  tags: Tags;

  appNames:
    | { type: 'loaded'; data: AppNames }
    | { type: 'reloading'; data: AppNames }
    | { type: 'failed'; data: AppNames };
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
    timeline: { type: 'pristine' },
    left: { type: 'pristine' },
    right: { type: 'pristine' },
  },
  tags: {},
  appNames: {
    type: 'loaded',
    data: (window as ShamefulAny).initialState.appNames,
  },
  query: appNameToQuery((window as ShamefulAny).initialState.appNames[0]) ?? '',
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

export const fetchComparisonSide = createAsyncThunk<
  { side: 'left' | 'right'; data: RenderOutput },
  { side: 'left' | 'right'; query: string },
  { state: { continuous: ContinuousState } }
>('continuous/fetchComparisonSide', async ({ side, query }, thunkAPI) => {
  const state = thunkAPI.getState();

  // We have to request the data for a given time range, to populate the flamegraph
  // And also request the timeline for the PARENT time range, to populate the timeline
  const res = await Promise.all(
    (() => {
      switch (side) {
        case 'left': {
          return [
            renderSingle({
              ...state.continuous,
              query,
            }),
            renderSingle({
              ...state.continuous,
              // TODO: what if there's no query? we should return nothing
              // maybe we should take the query as an action payload
              //              query: state.continuous.leftQuery || '',
              query,

              from: state.continuous.leftFrom,
              until: state.continuous.leftUntil,
            }),
          ];
        }
        case 'right': {
          return [
            renderSingle({
              ...state.continuous,
              query,
              //              query: state.continuous.rightQuery || '',
            }),
            renderSingle({
              ...state.continuous,

              // TODO
              //              query: state.continuous.rightQuery || '',
              query,
              from: state.continuous.rightFrom,
              until: state.continuous.rightUntil,
            }),
          ];
        }
        default: {
          throw new Error('Invalid side');
        }
      }
    })()
  );

  if (res[0].isOk && res[1].isOk) {
    return Promise.resolve({
      side,
      data: {
        timeline: res[0].value.timeline,
        profile: res[1].value.profile,
      },
    });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: `Failed to load the ${side} side comparison`,
      message: '',
      additionalInfo: [res[0].error.message, res[1].error.message],
    })
  );

  return Promise.reject(res.filter((a) => a.isErr).map((a) => a.error));
});

export const fetchDiffView = createAsyncThunk<
  RenderOutput,
  null,
  { state: { continuous: ContinuousState } }
>('continuous/diffView', async (_, thunkAPI) => {
  const state = thunkAPI.getState();
  const res = await renderDiff(state.continuous);

  if (res.isOk) {
    return Promise.resolve(res.value);
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
    builder.addCase(fetchComparisonSide.fulfilled, (state, action) => {
      state.comparisonView[action.meta.arg.side] = {
        ...action.payload.data,
        type: 'loaded',
      };
    });

    /***********************/
    /*      Diff View      */
    /***********************/
    builder.addCase(fetchDiffView.pending, (state) => {
      switch (state.diffView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
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

    builder.addCase(fetchDiffView.rejected, (state) => {
      switch (state.diffView.type) {
        // if previous state is loaded, let's continue displaying data
        case 'reloading': {
          state.diffView = {
            ...state.diffView,
            type: 'loaded',
          };
          break;
        }

        default: {
          // it failed to load for the first time, so far all effects it's pristine
          state.diffView = {
            type: 'pristine',
          };
        }
      }
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
    loadingStates.includes(state.continuous.diffView.type)
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
