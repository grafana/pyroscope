import { useEffect } from 'react';
import { useAppDispatch } from '@phlare/redux/hooks';
import { setQuery, reloadAppNames } from '@phlare/redux/reducers/continuous';
import { appToQuery } from '@phlare/models/app';

/**
 * loads and select the first app/type (if available)
 */
export function useSelectFirstApp() {
  const dispatch = useAppDispatch();

  useEffect(() => {
    async function run() {
      const apps = await dispatch(reloadAppNames()).unwrap();

      if (apps.length > 0) {
        // Select first app automatically
        dispatch(setQuery(appToQuery(apps[0])));
      }
    }

    run();
  }, [dispatch]);
}
