import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@phlare/redux/hooks';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@phlare/redux/reducers/user';
import { isAuthRequired } from '@phlare/util/features';
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
  }, [dispatch]);

  if (!isAuthRequired || currentUser) {
    return <>{children}</>;
  }

  return <></>;
}
