import { brandQuery, Query, queryToAppName } from '@webapp/models/query';
import type { RootState } from '@webapp/redux/store';
import { TagsState } from './state';

export const selectContinuousState = (state: RootState) => state.continuous;
export const selectApplicationName = (state: RootState) => {
  const { query } = selectQueries(state);

  const appName = queryToAppName(query);

  return appName.map((q) => q.split('{')[0]).unwrapOrElse(() => '');
};

export const selectAppNamesState = (state: RootState) =>
  state.continuous.appNames;
export const selectAppNames = (state: RootState) => {
  const sorted = [...state.continuous.appNames.data].sort();
  return sorted;
};

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
    loadingStates.includes(state.continuous.rightTimeline.type) ||
    // Exemplars
    loadingStates.includes(state.tracing.exemplarsSingleView.type) ||
    // Tag Explorer
    loadingStates.includes(
      state.continuous.tagExplorerView.groupsLoadingType
    ) ||
    loadingStates.includes(
      state.continuous.tagExplorerView.activeTagProfileLoadingType
    )
  );
};

export const selectAppTags = (query?: Query) => (state: RootState) => {
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

export const selectTimelineSides = (state: RootState) => {
  return {
    left: state.continuous.leftTimeline,
    right: state.continuous.rightTimeline,
  };
};

export const selectTimelineSidesData = (state: RootState) => {
  return {
    left: state.continuous.leftTimeline.timeline,
    right: state.continuous.rightTimeline.timeline,
  };
};

export const selectQueries = (state: RootState) => {
  return {
    leftQuery: brandQuery(state.continuous.leftQuery || ''),
    rightQuery: brandQuery(state.continuous.rightQuery || ''),
    query: brandQuery(state.continuous.query),
  };
};

// TODO: accept a side (continuous / leftside)
export const selectAnnotationsOrDefault = (state: RootState) => {
  if ('annotations' in state.continuous.singleView) {
    return state.continuous.singleView.annotations;
  }
  return [];
};

export const selectRanges = (rootState: RootState) => {
  const state = rootState.continuous;

  return {
    left: {
      from: state.leftFrom,
      until: state.leftUntil,
    },
    right: {
      from: state.rightFrom,
      until: state.rightUntil,
    },
    regular: {
      from: state.from,
      until: state.until,
    },
  };
};
