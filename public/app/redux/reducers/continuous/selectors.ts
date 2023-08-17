import { brandQuery, Query, queryToAppName } from '@pyroscope/models/query';
import type { RootState } from '@pyroscope/redux/store';
import { ContinuousState, TagsState } from './state';

export const selectContinuousState = (state: RootState) => state.continuous;
export const selectApplicationName = (state: RootState) => {
  const { query } = selectQueries(state);

  const appName = queryToAppName(query);

  return appName.map((q) => q.split('{')[0]).unwrapOrElse(() => '');
};

export const selectAppNamesState = (state: RootState) => state.continuous.apps;

/** Selected all applications and sort alphabetically by name */
export const selectApps = (state: RootState) => {
  // Shallow copy, since sort is in place
  return state.continuous.apps.data
    .slice(0)
    .sort((a, b) => a.name.localeCompare(b.name));
};

export const selectAppNames = (state: RootState) => {
  return selectApps(state).map((a) => a.name);
};

export const selectComparisonState = (state: RootState) =>
  state.continuous.comparisonView;

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

type ViewStates = Extract<keyof ContinuousState, `${string}View`>;
export const selectAnnotationsOrDefault =
  (view: ViewStates) => (state: RootState) => {
    switch (view) {
      case 'singleView': {
        if ('annotations' in state.continuous.singleView) {
          return state.continuous.singleView.annotations;
        }
        return [];
      }

      case 'tagExplorerView': {
        return state.continuous.tagExplorerView.annotations;
      }

      // Merge data from both sides into a single annotation
      // Which is fine, since this extra data won't be used if outside the time range
      // NOTE: this assumes the left and right timelines belong to the same application
      case 'diffView':
      case 'comparisonView': {
        const left =
          'annotations' in state.continuous.leftTimeline
            ? state.continuous.leftTimeline.annotations
            : [];
        const right =
          'annotations' in state.continuous.rightTimeline
            ? state.continuous.rightTimeline.annotations
            : [];
        return [...left, ...right];
      }

      default:
        const exhaustiveCheck: never = view;
        throw new Error(`Unhandled case: ${exhaustiveCheck}`);
    }
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
