import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  reloadAppNames,
  selectAppNames,
  setQuery,
  selectApplicationName,
} from '@webapp/redux/reducers/continuous';
import { appNameToQuery } from '@webapp/util/query';

export default function Continuous({
  children,
}: {
  children: React.ReactNode;
}) {
  const dispatch = useAppDispatch();
  const appNames = useAppSelector(selectAppNames);
  const selectedAppName = useAppSelector(selectApplicationName);

  useEffect(() => {
    async function loadAppNames() {
      if (appNames.length <= 0) {
        // Load application names
        const names = await dispatch(reloadAppNames());

        // Pick the first one if there's nothing selected
        if (!selectedAppName && names.payload.length > 0) {
          dispatch(setQuery(appNameToQuery(names.payload[0])));
        }
      }
    }

    loadAppNames();
  }, [dispatch, appNames, selectedAppName]);

  return children;
}
