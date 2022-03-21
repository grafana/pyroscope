import { Profile } from '@pyroscope/models';
import { shareWithFlamegraphDotcom } from '@pyroscope/services/share';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import handleError from '../util/handleError';

export default function useExportToFlamegraphDotCom(flamebearer?: Profile) {
  const dispatch = useAppDispatch();

  return async () => {
    if (!flamebearer) {
      return '';
    }

    const res = await shareWithFlamegraphDotcom({ flamebearer });

    if (res.isErr) {
      handleError(dispatch, 'Failed to export to flamegraph.com', res.error);
      return null;
    }

    return res.value.url;
  };
}
