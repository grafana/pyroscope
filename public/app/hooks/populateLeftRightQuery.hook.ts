import { useEffect } from 'react';
import { actions, selectQueries } from '@pyroscope/redux/reducers/continuous';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import { queryToAppName, Query } from '@pyroscope/models/query';

function isQueriesHasSameApp(queries: Query[]): boolean {
  const appName = queryToAppName(queries[0]);
  if (appName.isNothing) {
    return false;
  }

  return queries.every((query) => query.startsWith(appName.value));
}

// usePopulateLeftRightQuery populates the left and right queries using the main query
export default function usePopulateLeftRightQuery() {
  const dispatch = useAppDispatch();
  const { query, leftQuery, rightQuery } = useAppSelector(selectQueries);

  // should not populate queries when redirected
  // plus it prohibits different apps from being compared/diffed
  const shouldResetQuery =
    query && !isQueriesHasSameApp([query, leftQuery, rightQuery]);

  // When the query changes (ie the app has changed)
  // We populate left and right tags to reflect that application
  useEffect(() => {
    if (shouldResetQuery) {
      dispatch(actions.setRightQuery(query));
      dispatch(actions.setLeftQuery(query));
    }
  }, [dispatch, query, shouldResetQuery]);
}
