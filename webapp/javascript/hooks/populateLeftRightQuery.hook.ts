import { useEffect } from 'react';
import { actions, selectQueries } from '@webapp/redux/reducers/continuous';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';

// usePopulateLeftRightQuery populates the left and right queries using the main query
export default function usePopulateLeftRightQuery() {
  const dispatch = useAppDispatch();
  const { query } = useAppSelector(selectQueries);

  // When the query changes (ie the app has changed)
  // We populate left and right tags to reflect that application
  useEffect(() => {
    if (query) {
      dispatch(actions.setRightQuery(query));
      dispatch(actions.setLeftQuery(query));
    }
  }, [query]);
}
