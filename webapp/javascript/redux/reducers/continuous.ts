import { Profile } from '@pyroscope/models';
import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import { AppNames } from '@models/appNames';
import { fetchAppNames } from '@pyroscope/services/appNames';
import { appNameToQuery } from '@utils/query';
import { renderSingle, RenderOutput } from '../../services/render';
import { addNotification } from './notifications';
import { Timeline } from '../../models/timeline';
import * as tagsService from '../../services/tags';
import type { RootState } from '../store';

type DataState<T> =
  | {
      type: 'pristine';
    }
  | { type: 'loading' }
  | {
      type: 'loaded';
      data: T;
    }
  | {
      type: 'reloading';
      data: T;
    };

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

type ComparisonView =
  | { type: 'pristine' }
  | { type: 'loading' }
  | {
      type: 'loaded';
      timeline: Timeline;
      left: {
        profile: Profile;
        timeline: Timeline;
      };
      right: {
        profile: Profile;
        timeline: Timeline;
      };
    }
  | {
      type: 'reloading';
      timeline: Timeline;
      left: {
        profile: Profile;
        timeline: Timeline;
      };
      right: {
        profile: Profile;
        timeline: Timeline;
      };
    };

type TagsData =
  | { type: 'pristine' }
  | { type: 'loading' }
  | { type: 'failed' }
  | { type: 'loaded'; values: string[] };

// Tags really refer to each application
// Should we nest them to an application?
type Tags =
  | { type: 'pristine'; tags: Record<string, TagsData> }
  | { type: 'loading'; tags: Record<string, TagsData> }
  | {
      type: 'loaded';
      tags: Record<string, TagsData>;
    }
  | { type: 'failed'; tags: Record<string, TagsData> };

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
      title: 'Failed',
      message: `Failed to load singleView`,
    })
  );

  return Promise.reject(res.error);
});

// export const fetchSingleView = createAsyncThunk<
//  RenderOutput,
//  null,
//  { state: { continuous: ContinuousState } }
// >('continuous/singleView', async (_, thunkAPI) => {
//  const state = thunkAPI.getState();
//  const res = await renderSingle(state.continuous);
//
//  if (res.isOk) {
//    return Promise.resolve(res.value);
//  }
//
//  thunkAPI.dispatch(
//    addNotification({
//      type: 'danger',
//      title: 'Failed',
//      message: `Failed to load singleView`,
//    })
//  );
//
//  return Promise.reject(res.error);
// });

export const fetchTags = createAsyncThunk(
  'continuous/fetchTags',
  async (query: ContinuousState['query'], thunkAPI) => {
    const res = await tagsService.fetchTags(query);

    if (res.isOk) {
      return Promise.resolve(res.value);
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed',
        message: `Failed to load tags`,
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
    const res = await tagsService.fetchLabelValues(
      payload.label,
      payload.query
    );

    if (res.isOk) {
      return Promise.resolve({
        label: payload.label,
        values: res.value,
      });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed',
        message: `Failed to load tag values`,
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
        message: 'message' in res.error ? res.error.message : 'Unknown error',
      })
    );

    return Promise.reject(res.error);
  }
);

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
  comparisonView: { type: 'pristine' },
  tags: { type: 'pristine', tags: {} },
  appNames: { type: 'loaded', data: (window as any).initialState.appNames },
  query: appNameToQuery((window.initialState as any).appNames[0]) ?? '',
};

export const continuousSlice = createSlice({
  name: 'continuous',
  initialState,
  reducers: {
    setFrom(state, action: PayloadAction<string>) {
      state.from = action.payload;
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

    setDateRange(
      state,
      action: PayloadAction<Pick<ContinuousState, 'from' | 'until'>>
    ) {
      state.from = action.payload.from;
      state.until = action.payload.until;
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

    builder.addCase(fetchTags.pending, (state) => {
      state.tags = {
        ...state.tags,
        type: 'loading',
      };
    });

    builder.addCase(fetchTags.fulfilled, (state, action) => {
      const tags = action.payload.reduce((acc, tag) => {
        acc[tag] = { type: 'pristine' };
        return acc;
      }, {} as ContinuousState['tags']['tags']);

      state.tags = {
        type: 'loaded',
        tags,
      };
    });

    builder.addCase(fetchTags.rejected, (state) => {
      state.tags = {
        ...state.tags,
        type: 'failed',
      };
    });

    // TODO other cases
    builder.addCase(fetchTagValues.fulfilled, (state, action) => {
      if (state.tags.type !== 'loaded') {
        console.error('Loading tag values for an unloaded tags. Ignoring');
        return;
      }

      if (!state.tags.tags[action.payload.label]) {
        // We are loading tag values for a non existent tag
        console.error(
          `Loaded labels values for non existing label: '${action.payload.label}'. Ignoring`
        );
        return;
      }

      state.tags.tags[action.payload.label] = {
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
export const selectLabelsList = (state: RootState) => {
  return Object.keys(state.continuous.tags.tags);
};
export const selectLabels = (state: RootState) => {
  return state.continuous.tags.tags;
};
export const selectApplicationName = (state: RootState) => {
  return state.continuous.query.split('{')[0];
};

export const selectAppNamesState = (state: RootState) =>
  state.continuous.appNames;
export const selectAppNames = (state: RootState) =>
  state.continuous.appNames.data;
