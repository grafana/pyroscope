import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@pyroscope/redux/reducers/user';
import { useHistory } from 'react-router-dom';
import { PAGES } from '@pyroscope/pages/urls';

export default function useNavigateUserIntroPages() {
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const history = useHistory();

  // loading user on page mount
  useEffect(() => {
    dispatch(loadCurrentUser());
  }, [dispatch]);
  // there are cases when user doesn't exist on page mount
  // but appears after submitting login/signup form
  useEffect(() => {
    if (currentUser) {
      history.push(PAGES.CONTINOUS_SINGLE_VIEW);
    }
  }, [history, currentUser]);
}
