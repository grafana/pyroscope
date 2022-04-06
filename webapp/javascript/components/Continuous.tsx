import React, { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  reloadAppNames,
  selectAppNames,
  setQuery,
} from '@webapp/redux/reducers/continuous';

export default function Continuous({
  children,
}: {
  children: React.ReactNode;
}) {
  const dispatch = useAppDispatch();
  const appNames = useAppSelector(selectAppNames);

  useEffect(() => {
    async function loadAppNames() {
      if (appNames.length <= 0) {
        // Load application names
        const names = await dispatch(reloadAppNames());

        // Pick the first one
        if (names.payload.length > 0) {
          dispatch(setQuery(names.payload[0]));
        }
      }
    }

    loadAppNames();
  }, [dispatch, appNames]);

  return children;
}
