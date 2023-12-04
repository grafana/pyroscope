import { useEffect } from 'react';
import { actions as tracingActions } from '@pyroscope/redux/reducers/tracing';
import { history } from '@pyroscope/util/history';
import ReduxQuerySync from 'redux-query-sync';
import { actions as continuousActions } from './reducers/continuous';
import store, { RootState } from './store';

export function setupReduxQuerySync() {
  // This is a bi-directional sync between the query parameters and the redux store
  // It works as follows:
  // * When URL query changes, It will dispatch the action
  // * When the store changes (the field set in selector), the query param is updated
  // For more info see the implementation at
  // https://github.com/Treora/redux-query-sync/blob/master/src/redux-query-sync.js
  return ReduxQuerySync({
    store,
    params: {
      from: {
        defaultValue: 'now-1h',
        selector: (state: RootState) => state.continuous.from,
        action: continuousActions.setFrom,
      },
      until: {
        defaultValue: 'now',
        selector: (state: RootState) => state.continuous.until,
        action: continuousActions.setUntil,
      },
      leftFrom: {
        defaultValue: 'now-1h',
        selector: (state: RootState) => state.continuous.leftFrom,
        action: continuousActions.setLeftFrom,
      },
      leftUntil: {
        defaultValue: 'now-30m',
        selector: (state: RootState) => state.continuous.leftUntil,
        action: continuousActions.setLeftUntil,
      },
      rightFrom: {
        defaultValue: 'now-30m',
        selector: (state: RootState) => state.continuous.rightFrom,
        action: continuousActions.setRightFrom,
      },
      rightUntil: {
        defaultValue: 'now',
        selector: (state: RootState) => state.continuous.rightUntil,
        action: continuousActions.setRightUntil,
      },
      query: {
        defaultvalue: '',
        selector: (state: RootState) => {
          const {
            continuous: { query },
          } = state;
          // Only sync the query URL parameter if it is actually set to something
          // Otherwise `?query=` will always be appended to the URL
          if (query !== '') {
            return query;
          }
          return undefined;
        },
        action: continuousActions.setQuery,
      },
      queryID: {
        defaultvalue: '',
        selector: (state: RootState) => state.tracing.queryID,
        action: tracingActions.setQueryID,
      },
      rightQuery: {
        defaultvalue: '',
        selector: (state: RootState) => state.continuous.rightQuery,
        action: continuousActions.setRightQuery,
      },
      leftQuery: {
        defaultvalue: '',
        selector: (state: RootState) => state.continuous.leftQuery,
        action: continuousActions.setLeftQuery,
      },
      maxNodes: {
        defaultValue: '0',
        selector: (state: RootState) => state.continuous.maxNodes,
        action: continuousActions.setMaxNodes,
      },
      aggregation: {
        defaultValue: 'sum',
        selector: (state: RootState) => state.continuous.aggregation,
        action: continuousActions.setAggregation,
      },
      groupBy: {
        defaultValue: '',
        selector: (state: RootState) =>
          state.continuous.tagExplorerView.groupByTag,
        action: continuousActions.setTagExplorerViewGroupByTag,
      },
      groupByValue: {
        defaultValue: '',
        selector: (state: RootState) =>
          state.continuous.tagExplorerView.groupByTagValue,
        action: continuousActions.setTagExplorerViewGroupByTagValue,
      },
    },
    initialTruth: 'location',
    replaceState: false,
    history,
  });
}

// setup & unsubscribe on unmount
export function useReduxQuerySync() {
  useEffect(setupReduxQuerySync, []);
}
