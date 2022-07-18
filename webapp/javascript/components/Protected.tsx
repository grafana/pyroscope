import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@webapp/redux/reducers/user';
import { useHistory, useLocation } from 'react-router-dom';

export default function Protected({ children }: { children: React.ReactNode }) {
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const history = useHistory();
  const location = useLocation();
  const { isAuthRequired } = window as ShamefulAny;

  useEffect(() => {
    if (isAuthRequired) {
      dispatch(loadCurrentUser()).then((e: ShamefulAny): void => {
        if (!e.isOk && e?.error?.code === 401) {
          history.push('/login', { redir: location });
        }
      });
    }
  }, [dispatch]);

  return !isAuthRequired || currentUser ? children : null;
}
