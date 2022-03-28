import { useEffect } from 'react';
import {
  actions,
  selectContinuousState,
} from '@webapp/redux/reducers/continuous';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';

// usePopulateLeftRightQuery populates the left and right queries using the main query
export default function usePopulateLeftRightQuery() {
  const dispatch = useAppDispatch();
  const { query, leftQuery, rightQuery } = useAppSelector(
    selectContinuousState
  );

  // When the query changes (ie the app has changed)
  // We populate left and right tags to reflect that application
  useEffect(() => {
    if (query && !rightQuery) {
      dispatch(actions.setRightQuery(query));
    }
    if (query && !leftQuery) {
      dispatch(actions.setLeftQuery(query));
    }
  }, [query]);
}
