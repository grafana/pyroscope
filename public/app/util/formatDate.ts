/* eslint-disable no-underscore-dangle */
import { add, format, getUnixTime } from 'date-fns';

const multiplierMapping = new Map(
  Object.entries({
    s: 'seconds',
    m: 'minutes',
    h: 'hours',
    d: 'days',
    w: 'weeks',
    M: 'months',
    y: 'years',
  })
);

export function convertPresetsToDate(from: string) {
  const match = from.match(/^now-(?<number>\d+)(?<multiplier>\D+)$/);
  if (!match) {
    throw new Error(`Could not apply regex to '${from}'`);
  }

  const { groups } = match;
  if (!groups) {
    throw new Error(`Could not extract required fields from regex'`);
  }

  const { number, multiplier } = groups;

  const _multiplier = multiplierMapping.get(multiplier);
  if (!_multiplier) {
    throw new Error(`Cant access ${multiplier} from map`);
  }
  const now = new Date();

  const _from =
    add(now, {
      [_multiplier]: -number,
    }).getTime() / 1000;

  return { _from, number, _multiplier };
}

export function readableRange(
  from: string,
  until: string,
  offsetInMinutes: number
) {
  const dateFormat = 'yyyy-MM-dd hh:mm a';
  if (/^now-/.test(from) && until === 'now') {
    const { number, _multiplier } = convertPresetsToDate(from);
    const multiplier =
      parseInt(number, 10) >= 2 ? _multiplier : _multiplier.slice(0, -1);
    return `Last ${number} ${multiplier}`;
  }

  const d1 = getUTCdate(parseUnixTime(from), offsetInMinutes);
  const d2 = getUTCdate(parseUnixTime(until), offsetInMinutes);

  if (!isValidDate(d1) || !isValidDate(d2)) {
    return '';
  }

  return `${format(d1, dateFormat)} - ${format(d2, dateFormat)}`;
}

function isValidDate(d: Date) {
  return d instanceof Date && !isNaN(d.getTime());
}

/**
 * formateAsOBject() returns a Date object
 * based on the passed-in parameter value
 * which is either a Number repsenting a date
 * or a default preset(example: "now-1h")
 * this is necessary because the DatePicker component
 * from react-datepicker package has a property (selected)
 * that requires an input of type Date if we passed another
 * type the Component will throw an error and the app will crash
 */
export function formatAsOBject(value: string) {
  if (/^now-/.test(value)) {
    const { _from } = convertPresetsToDate(value);
    return new Date(_from * 1000);
  }
  if (value === 'now') {
    return new Date();
  }
  return parseUnixTime(value);
}

export function parseUnixTime(value: string) {
  const parsed = parseInt(value, 10);
  switch (value.length) {
    default:
      // Seconds.
      return new Date(parsed * 1000);
    case 13: // Milliseconds.
      return new Date(parsed);
    case 16: // Microseconds.
      return new Date(Math.round(parsed / 1000));
    case 19: // Nanoseconds.
      return new Date(Math.round(parsed / 1000 / 1000));
  }
}

export const getUTCdate = (date: Date, offsetInMinutes: number): Date =>
  offsetInMinutes === 0
    ? new Date(date.getTime() + date.getTimezoneOffset() * 60 * 1000)
    : date;

export const getTimelineFormatDate = (date: Date, hours: number) => {
  if (hours < 12) {
    return format(date, 'HH:mm:ss');
  }
  if (hours >= 12 && hours <= 24) {
    return format(date, 'HH:mm');
  }
  if (hours > 24) {
    return format(date, 'MMM do HH:mm');
  }
  return format(date, 'MMM do HH:mm');
};

export function timezoneToOffset(timezone: 'utc' | 'browser'): number {
  if (timezone === 'utc') {
    return 0;
  }

  // Use browser's
  // FIXME: this does not account for arbitrary timezones
  // eg one that is not the user's browser
  return new Date().getTimezoneOffset();
}

/**
 * given a Date returns its representation in unix timestamp
 */
export function toUnixTimestamp(d: Date) {
  return getUnixTime(d);
}
