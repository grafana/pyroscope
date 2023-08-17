import { Profile } from '@pyroscope/legacy/models';
import { shareWithFlamegraphDotcom } from '@pyroscope/services/share';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import handleError from '@pyroscope/util/handleError';

export default function useExportToFlamegraphDotCom(
  flamebearer?: Profile,
  groupByTag?: string,
  groupByTagValue?: string
) {
  const dispatch = useAppDispatch();

  return async (name?: string) => {
    if (!flamebearer) {
      return '';
    }

    const res = await shareWithFlamegraphDotcom({
      flamebearer,
      name,
      // or we should add this to name ?
      groupByTag,
      groupByTagValue,
    });

    if (res.isErr) {
      handleError(dispatch, 'Failed to export to flamegraph.com', res.error);
      return null;
    }

    return res.value.url;
  };
}
