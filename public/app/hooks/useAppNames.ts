import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  setQuery,
  reloadAppNames,
  selectQueries,
} from '@pyroscope/redux/reducers/continuous';
import { appToQuery } from '@pyroscope/models/app';

/**
 * loads and select the first app/type (if available, if needed)
 */
export function useSelectFirstApp() {
  const dispatch = useAppDispatch();

  const { query } = useAppSelector(selectQueries);

  useEffect(() => {
    async function run() {
      const apps = await dispatch(reloadAppNames()).unwrap();

      if (apps.length > 0 && query === '') {
        // Select first app automatically if there is no query selected
        dispatch(setQuery(appToQuery(apps[0])));
      }
    }

    run();
  }, [dispatch, query]);
}
