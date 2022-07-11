import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  reloadAppNames,
  selectAppNames,
  setQuery,
  selectApplicationName,
} from '@webapp/redux/reducers/continuous';
import { queryFromAppName } from '@webapp/models/query';

export default function Continuous({
  children,
}: {
  children: React.ReactNode;
}) {
  const dispatch = useAppDispatch();
  const appNames = useAppSelector(selectAppNames);
  const selectedAppName = useAppSelector(selectApplicationName);

  useEffect(() => {
    async function run() {
      await dispatch(reloadAppNames());
    }

    run();
  }, [dispatch]);

  // Pick the first one if there's nothing selected
  useEffect(() => {
    if (!selectedAppName && appNames.length > 0) {
      dispatch(setQuery(queryFromAppName(appNames[0])));
    }
  }, [appNames, selectedAppName]);

  return children;
}
