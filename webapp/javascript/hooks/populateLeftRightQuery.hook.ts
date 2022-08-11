import { useEffect } from 'react';
import { actions, selectQueries } from '@webapp/redux/reducers/continuous';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';

function isQueriesHasSameApp(queries: string[]): boolean {
  const appName = queries[0].split('{')[0];

  return queries.every((query) => query.match(appName));
}

// usePopulateLeftRightQuery populates the left and right queries using the main query
export default function usePopulateLeftRightQuery() {
  const dispatch = useAppDispatch();
  const { query, leftQuery, rightQuery } = useAppSelector(selectQueries);
  // should not populate queries when was redirected
  const shouldResetQuery =
    query && !isQueriesHasSameApp([query, leftQuery, rightQuery]);

  // When the query changes (ie the app has changed)
  // We populate left and right tags to reflect that application
  useEffect(() => {
    if (shouldResetQuery) {
      dispatch(actions.setRightQuery(query));
      dispatch(actions.setLeftQuery(query));
    }
  }, [shouldResetQuery]);
}
