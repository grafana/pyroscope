/* eslint-disable no-underscore-dangle */
// we import moment/src/moment instead of moment because we don't want to load locales
import moment from "moment/src/moment";

const multiplierMapping = {
  s: "second",
  m: "minute",
  h: "hour",
  d: "day",
  w: "week",
  M: "month",
  y: "year",
};

export function convertPresetsToDate(from) {
  const { groups } = from.match(/^now-(?<number>\d+)(?<multiplier>\D+)$/);
  const { number, multiplier } = groups;
  let _multiplier = multiplierMapping[multiplier];
  if (number > 1) {
    _multiplier += "s";
  }
  const _from = moment().add(-number, _multiplier).toDate() / 1000;

  return { _from, number, _multiplier };
}

export function readableRange(from, until) {
  const dateFormat = "YYYY-MM-DD hh:mm A";
  if (/^now-/.test(from) && until === "now") {
    const { number, _multiplier } = convertPresetsToDate(from);
    return `Last ${number} ${_multiplier}`;
  }

  if (until === "now" && !/^now-/.test(from)) {
    return `${moment(Math.round(from * 1000)).format(dateFormat)} - now`;
  }

  if (until !== "now" && /^now-/.test(from)) {
    const { _from } = convertPresetsToDate(from);
    return `${moment(Math.round(_from * 1000)).format(dateFormat)} - ${moment(
      Math.round(until * 1000)
    ).format(dateFormat)}`;
  }

  return `${moment(Math.round(from * 1000)).format(dateFormat)} - ${moment(
    Math.round(until * 1000)
  ).format(dateFormat)}`;
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
  if (value === "now") {
    return moment().toDate();
  }
  return moment(value * 1000).toDate();
}
