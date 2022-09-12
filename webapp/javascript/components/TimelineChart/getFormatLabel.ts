import { getUTCdate } from '@webapp/util/formatDate';
import { format } from 'date-fns';

function getFormatLabel({
  date,
  timezone,
  xaxis,
}: {
  date: number;
  timezone: string;
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

    if (hours < 12) {
      return format(d, 'HH:mm:ss');
    }
    if (hours >= 12 && hours <= 24) {
      return format(d, 'HH:mm');
    }
    if (hours > 24) {
      return format(d, 'MMM do HH:mm');
    }
    return format(d, 'MMM do HH:mm');
  } catch (e) {
    return '???';
  }
}

export default getFormatLabel;
