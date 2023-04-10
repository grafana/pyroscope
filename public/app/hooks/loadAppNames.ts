import { useEffect } from 'react';
import { useAppDispatch } from '@webapp/redux/hooks';
import { reloadAppNames } from '@webapp/redux/reducers/continuous';

export function loadAppNames() {
  const dispatch = useAppDispatch();

  useEffect(() => {
    async function run() {
      await dispatch(reloadAppNames());
    }

    run();
  }, [dispatch]);
}
