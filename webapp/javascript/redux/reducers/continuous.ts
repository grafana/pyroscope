import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import { fetchAppNames } from '@webapp/services/appNames';
import { Query } from '@webapp/models/query';
import { defaultcomparisonPeriod } from '@webapp/components/SideTimelineComparator/periods';
import { addNotification } from './notifications';
import { createAsyncThunk } from '../async-thunk';
import { ContinuousState, TagsState } from './continuous/state';
import { fetchTagValues, fetchTags } from './continuous/tags.thunks';
import { addAnnotation } from './continuous/annotations.thunks';
import { fetchSingleView } from './continuous/singleView.thunks';
import { fetchComparisonSide } from './continuous/comparisonView.thunks';
import { fetchSideTimelines } from './continuous/timelines.thunks';
import {
  fetchTagExplorerView,
  fetchTagExplorerViewProfile,
  ALL_TAGS,
} from './continuous/tagExplorer.thunks';
import { fetchDiffView } from './continuous/diffView.thunks';

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
    comparisonMode: {
      active: false,
      period: defaultcomparisonPeriod,
    },
  },
  tags: {},
  tagExplorerView: {
    groupByTag: '',
    groupByTagValue: '',
    type: 'pristine',
    groups: {},
  },
  newAnnotation: { type: 'pristine' },

  appNames: {
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
    setQuery(state, action: PayloadAction<Query>) {
      // TODO: figure out why is being dispatched as undefined
      state.query = action.payload || '';

      state.tagExplorerView.groupByTag = '';
      state.tagExplorerView.groupByTagValue = '';
    },
    setTagExplorerViewGroupByTag(state, action: PayloadAction<string>) {
      state.tagExplorerView.groupByTag = action.payload;
      state.tagExplorerView.groupByTagValue = ALL_TAGS;
    },
    setTagExplorerViewGroupByTagValue(state, action: PayloadAction<string>) {
      state.tagExplorerView.groupByTagValue = action.payload;
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

    setComparisonMode(state, action) {
      state.comparisonView.comparisonMode = action.payload;
    },
  },

  extraReducers: (builder) => {
    /**********************/
    /* GENERAL GUIDELINES */
    /**********************/

    // There are (currently) only 2 ways an action can be aborted:
    // 1. The component is unmounting, eg when changing route
    // 2. New data is loading, which means previous request is going to be superseeded
    // In both cases, not doing state transitions is fine
    // Specially in the second case, where a 'rejected' may happen AFTER a 'pending' is dispatched
    // https://redux-toolkit.js.org/api/createAsyncThunk#checking-if-a-promise-rejection-was-from-an-error-or-cancellation

    /*************************/
    /*      Single View      */
    /*************************/
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

    builder.addCase(fetchComparisonSide.rejected, (state, action) => {
      const { side } = action.meta.arg;

      if (action.meta.aborted) {
        return;
      }

      state.comparisonView[side] = {
        type: 'pristine',
      };
    });

    /*****************************/
    /*      Timeline Sides       */
    /*****************************/
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
        timeline: action.payload.left,
      };
      state.rightTimeline = {
        type: 'loaded',
        timeline: action.payload.right,
      };
    });

    // TODO
    builder.addCase(fetchSideTimelines.rejected, () => {});

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
    builder.addCase(fetchDiffView.rejected, (state, action) => {
      if (action.meta.aborted) {
        return;
      }

      state.diffView = {
        type: 'pristine',
      };
    });

    /*******************************/
    /*      Tag Explorer View      */
    /*******************************/

    builder.addCase(fetchTagExplorerView.pending, (state) => {
      switch (state.diffView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
        case 'reloading':
        case 'loaded': {
          state.tagExplorerView = {
            ...state.tagExplorerView,
            type: 'reloading',
          };
          break;
        }

        default: {
          state.tagExplorerView = {
            ...state.tagExplorerView,
            type: 'loading',
          };
        }
      }
    });

    builder.addCase(fetchTagExplorerView.fulfilled, (state, action) => {
      state.tagExplorerView = {
        ...state.tagExplorerView,
        ...action.payload,
        activeTagProfile: action.payload.profile,
        type: 'loaded',
      };
    });

    builder.addCase(fetchTagExplorerView.rejected, () => {});

    /***************************************/
    /*      Tag Explorer View Profile      */
    /***************************************/

    builder.addCase(fetchTagExplorerViewProfile.pending, () => {});

    builder.addCase(fetchTagExplorerViewProfile.fulfilled, (state, action) => {
      state.tagExplorerView = {
        ...state.tagExplorerView,
        activeTagProfile: action.payload.profile,
        type: 'loaded',
      };
    });

    builder.addCase(fetchTagExplorerViewProfile.rejected, () => {});

    /*****************/
    /*      Tags     */
    /*****************/

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

    /*****************/
    /*  Annotation   */
    /*****************/
    builder.addCase(addAnnotation.fulfilled, (state, action) => {
      // TODO(eh-am): allow arbitrary views
      if ('annotations' in state.singleView) {
        // TODO(eh-am): dedup
        state.singleView.annotations = [
          ...state.singleView.annotations,
          action.payload.annotation,
        ];
      }
      state.newAnnotation = { type: 'pristine' };
    });
    builder.addCase(addAnnotation.pending, (state) => {
      state.newAnnotation = { type: 'saving' };
    });
    builder.addCase(addAnnotation.rejected, (state) => {
      state.newAnnotation = { type: 'pristine' };
    });
  },
});

export default continuousSlice.reducer;
export const { actions } = continuousSlice;
export const { setDateRange, setQuery } = continuousSlice.actions;

function getNextStateFromPending(
  prevState: 'pristine' | 'loading' | 'reloading' | 'loaded'
) {
  if (prevState === 'pristine' || prevState === 'loading') {
    return 'loading';
  }

  return 'reloading';
}

export * from './continuous/selectors';

export * from './continuous/state';
export * from './continuous/tags.thunks';
export * from './continuous/singleView.thunks';
export * from './continuous/annotations.thunks';
export * from './continuous/comparisonView.thunks';
export * from './continuous/timelines.thunks';
export * from './continuous/tagExplorer.thunks';
export * from './continuous/diffView.thunks';
