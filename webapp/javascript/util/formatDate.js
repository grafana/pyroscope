/* eslint-disable no-underscore-dangle */
import { add, format } from 'date-fns';

const multiplierMapping = {
  s: 'second',
  m: 'minute',
  h: 'hour',
  d: 'day',
  w: 'week',
  M: 'month',
  y: 'year',
};

export function convertPresetsToDate(from) {
  const { groups } = from.match(/^now-(?<number>\d+)(?<multiplier>\D+)$/);
  const { number, multiplier } = groups;
  let _multiplier = multiplierMapping[multiplier];
  if (number > 1) {
    _multiplier += 's';
  }

  const _from =
    add(new Date(), {
      [multiplier + 's']: -number,
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
