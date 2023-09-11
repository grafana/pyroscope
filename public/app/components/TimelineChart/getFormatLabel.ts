import { getUTCdate, getTimelineFormatDate } from '@pyroscope/util/formatDate';

function getFormatLabel({
  date,
  timezone,
  xaxis,
}: {
  date: number;
  timezone?: string;
  xaxis: {
    min: number;
    max: number;
  };
}) {
  if (!xaxis) {
    return '';
  }

  try {
    const d = getUTCdate(
      new Date(date),
      timezone === 'utc' ? 0 : new Date().getTimezoneOffset()
    );

    const hours = Math.abs(xaxis.max - xaxis.min) / 60 / 60 / 1000;

    return getTimelineFormatDate(d, hours);
  } catch (e) {
    return '???';
  }
}

export default getFormatLabel;
