/* eslint-disable no-underscore-dangle */
import { add, format } from 'date-fns';

const multiplierMapping = {
  s: 'seconds',
  m: 'minutes',
  h: 'hours',
  d: 'days',
  w: 'weeks',
  M: 'months',
  y: 'years',
};

export function convertPresetsToDate(from) {
  const { groups } = from.match(/^now-(?<number>\d+)(?<multiplier>\D+)$/);
  const { number, multiplier } = groups;
  let _multiplier = multiplierMapping[multiplier];

  const now = new Date();

  const _from =
    add(now, {
      [_multiplier]: -number,
    }) / 1000;

  return { _from, number, _multiplier };
}

export function readableRange(from, until) {
  const dateFormat = 'yyyy-MM-dd hh:mm a';
  if (/^now-/.test(from) && until === 'now') {
    const { number, _multiplier } = convertPresetsToDate(from);
    return `Last ${number} ${_multiplier}`;
  }

  const d1 = new Date(Math.round(from * 1000));
  const d2 = new Date(Math.round(until * 1000));
  return `${format(d1, dateFormat)} - ${format(d2, dateFormat)}`;
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
export function formatAsOBject(value) {
  if (/^now-/.test(value)) {
    const { _from } = convertPresetsToDate(value);
    return _from * 1000;
  }
  if (value === 'now') {
    return new Date();
  }

  return new Date(value * 1000);
}
