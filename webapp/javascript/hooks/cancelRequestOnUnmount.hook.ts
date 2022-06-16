/* eslint-disable no-plusplus, react-hooks/exhaustive-deps */
import { useEffect } from 'react';

export default function useCancelRequestOnUnmount(
  requests: Array<{ abort?: (arg?: string) => void }>
) {
  useEffect(() => {
    return () => {
      for (let i = 0; i < requests.length; i++) {
        const f = requests?.[i];
        if (f?.abort) {
          f.abort('unmount');
        }
      }
    };
  }, []);
}
