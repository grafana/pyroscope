import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  reloadAppNames,
  selectAppNames,
  setQuery,
  selectApplicationName,
} from '@webapp/redux/reducers/continuous';
import { queryFromAppName } from '@webapp/models/query';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@webapp/redux/reducers/user';
import { useHistory, useLocation } from 'react-router-dom';

export default function Continuous({
  children,
}: {
  children: React.ReactNode;
}) {
  const dispatch = useAppDispatch();
  const appNames = useAppSelector(selectAppNames);
  const selectedAppName = useAppSelector(selectApplicationName);
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

  useEffect(() => {
    async function loadAppNames() {
      if (appNames.length <= 0) {
        // TODO(shaleynikov): split this into two components
        try {
          // Load application names
          const names = await dispatch(reloadAppNames()).unwrap();

          // Pick the first one if there's nothing selected
          if (!selectedAppName && names.length > 0) {
            dispatch(setQuery(queryFromAppName(names[0])));
          }
        } catch (e) {
          console.error(e);
        }
      }
    }

    loadAppNames();
  }, [dispatch, appNames, selectedAppName]);

  return !isAuthRequired || currentUser ? children : null;
}
