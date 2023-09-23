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
        // Select a reasonable default app automatically if there is no query selected

        // First, find a `cpu` type
        const cpuApp = apps.find((app)=>app.__profile_type__.split(":")[1] === 'cpu')

        // If we can't find a `cpu` type, just choose the top of the list
        const app = cpuApp ? cpuApp : apps[0];

        dispatch(setQuery(appToQuery(app)));
      }
    }

    run();
  }, [dispatch, query]);
}
