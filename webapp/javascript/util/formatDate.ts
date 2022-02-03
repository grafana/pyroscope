/* eslint-disable no-underscore-dangle */
import { add, format } from 'date-fns';

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

export function readableRange(from: string, until: string) {
  const dateFormat = 'yyyy-MM-dd hh:mm a';
  if (/^now-/.test(from) && until === 'now') {
    const { number, _multiplier } = convertPresetsToDate(from);
    return `Last ${number} ${_multiplier}`;
  }

  const d1 = new Date(Math.round(parseInt(from, 10) * 1000));
  const d2 = new Date(Math.round(parseInt(until, 10) * 1000));
  return `${format(d1, dateFormat)} - ${format(d2, dateFormat)}`;
}

export function dateForExportFilename(from: string, until: string) {
  const dateFormat = 'yyyy-MM-dd_HHmm';
  if (/^now-/.test(from) && until === 'now') {
    const { number, _multiplier } = convertPresetsToDate(from);
    return `Last ${number} ${_multiplier}`;
  }

  const d1 = new Date(Math.round(parseInt(from, 10) * 1000));
  const d2 = new Date(Math.round(parseInt(until, 10) * 1000));
  return `${format(d1, dateFormat)}-to-${format(d2, dateFormat)}`;
}
/**
 * formateAsOBject() returns a Date object
 * based on the passed-in parameter value
 * which is either a Number repsenting a date
 * or a default preset(example: "now-1h")
 * this is neccessary because the DatePicker component
 * from react-datepicker package has a property (selected)
 * that requires an input of type Date if we passed another
 * type the Component will throw an error and the app will crash
 */
export function formatAsOBject(value: string) {
  if (/^now-/.test(value)) {
    const { _from } = convertPresetsToDate(value);
    return _from * 1000;
  }
  if (value === 'now') {
    return new Date().getTime();
  }

  return new Date(parseInt(value, 10) * 1000).getTime();
}
