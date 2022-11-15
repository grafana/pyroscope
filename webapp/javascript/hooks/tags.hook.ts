import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  fetchTags,
  selectRanges,
  selectQueries,
  selectAppTags,
} from '@webapp/redux/reducers/continuous';

// useTags handle loading tags when any of the following changes
// query, from, until
// Since the backend may have new tags given a new interval
export default function useTags() {
  const dispatch = useAppDispatch();
  const ranges = useAppSelector(selectRanges);
  const queries = useAppSelector(selectQueries);
  const { leftQuery, rightQuery, query } = queries;

  const regularTags = useAppSelector(selectAppTags(query));
  const leftTags = useAppSelector(selectAppTags(leftQuery));
  const rightTags = useAppSelector(selectAppTags(rightQuery));

  useEffect(() => {
    if (leftQuery) {
      dispatch(fetchTags(leftQuery));
    }
  }, [leftQuery, JSON.stringify(ranges.left)]);

  useEffect(() => {
    if (rightQuery) {
      dispatch(fetchTags(rightQuery));
    }
  }, [rightQuery, JSON.stringify(ranges.right)]);

  useEffect(() => {
    if (query) {
      dispatch(fetchTags(query));
    }
  }, [query, JSON.stringify(ranges.regular)]);

  return {
    regularTags,
    leftTags,
    rightTags,
  };
}
