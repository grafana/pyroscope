import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@pyroscope/redux/reducers/user';
import { isAuthRequired } from '@pyroscope/util/features';
import { useHistory, useLocation } from 'react-router-dom';

export default function Protected({
  children,
}: {
  children: React.ReactElement | React.ReactElement[];
}): JSX.Element {
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const history = useHistory();
  const location = useLocation();

  useEffect(() => {
    if (isAuthRequired) {
      dispatch(loadCurrentUser()).then((e: ShamefulAny): void => {
        if (!e.isOk && e?.error?.code === 401) {
          history.push('/login', { redir: location });
        }
      });
    }
  }, [dispatch, history, location]);

  if (!isAuthRequired || currentUser) {
    return <>{children}</>;
  }

  return <></>;
}
