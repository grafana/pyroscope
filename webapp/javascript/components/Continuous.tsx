import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  reloadAppNames,
  selectAppNames,
  setQuery,
  selectApplicationName,
} from '@webapp/redux/reducers/continuous';
import { queryFromAppName } from '@webapp/models/query';
import { loadCurrentUser } from '@webapp/redux/reducers/user';

export default function Continuous({
  children,
}: {
  children: React.ReactNode;
}) {
  const dispatch = useAppDispatch();
  const appNames = useAppSelector(selectAppNames);
  const selectedAppName = useAppSelector(selectApplicationName);

  useEffect(() => {
    if ((window as ShamefulAny).isAuthRequired) {
      dispatch(loadCurrentUser());
    }
  }, [dispatch]);

  useEffect(() => {
    async function loadAppNames() {
      if (appNames.length <= 0) {
        // Load application names
        const names = await dispatch(reloadAppNames()).unwrap();

        // Pick the first one if there's nothing selected
        if (!selectedAppName && names.length > 0) {
          dispatch(setQuery(queryFromAppName(names[0])));
        }
      }
    }

    loadAppNames();
  }, [dispatch, appNames, selectedAppName]);

  return children;
}
