import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { selectAppTags, fetchTags } from '@webapp/redux/reducers/continuous';

// useTags handle loading tags when query changes
export default function useTags({
  leftQuery,
  rightQuery,
}: {
  leftQuery?: string;
  rightQuery?: string;
}) {
  const dispatch = useAppDispatch();
  const leftTags = useAppSelector(selectAppTags(leftQuery));
  const rightTags = useAppSelector(selectAppTags(rightQuery));

  useEffect(() => {
    // if they are both the same, just load once
    if (leftQuery && rightQuery && leftQuery === rightQuery) {
      dispatch(fetchTags(leftQuery));
      return;
    }

    if (leftQuery) {
      dispatch(fetchTags(leftQuery));
    }
    if (rightQuery) {
      dispatch(fetchTags(rightQuery));
    }

    // TODO: cancellation
  }, [leftQuery, rightQuery]);

  return {
    leftTags,
    rightTags,
  };
}
