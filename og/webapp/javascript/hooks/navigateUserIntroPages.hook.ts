import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@webapp/redux/reducers/user';
import { useHistory } from 'react-router-dom';
import { PAGES } from '@webapp/pages/constants';

export default function useNavigateUserIntroPages() {
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const history = useHistory();

  // loading user on page mount
  useEffect(() => {
    dispatch(loadCurrentUser());
  }, []);
  // there are cases when user doesn't exist on page mount
  // but appears after submitting login/signup form
  useEffect(() => {
    if (currentUser) {
      history.push(PAGES.CONTINOUS_SINGLE_VIEW);
    }
  }, [currentUser]);
}
