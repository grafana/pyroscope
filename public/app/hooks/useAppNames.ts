import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  setQuery,
  reloadAppNames,
  selectQueries,
} from '@pyroscope/redux/reducers/continuous';
import { appToQuery } from '@pyroscope/models/app';
import { determineDefaultApp } from '@pyroscope/hooks/util/determineDefaultApp';

/**
 * loads and select the first app/type (if available, if needed)
 */
export function useSelectFirstApp() {
  const dispatch = useAppDispatch();

  const { query } = useAppSelector(selectQueries);

  useEffect(() => {
    async function run() {
      const apps = await dispatch(reloadAppNames()).unwrap();

      if (!apps.length || query) {
        return;
      }

      const defaultApp = await determineDefaultApp(apps);

      dispatch(setQuery(appToQuery(defaultApp)));
    }

    run();
  }, [dispatch, query]);
}
