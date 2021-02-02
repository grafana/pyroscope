/* eslint-disable no-underscore-dangle */
import moment from "moment";

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
  const dateFormat = "YYYY-DD-MM hh:mm A";
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
 * taking state.from and state.until values
 * and converting it to a date object
 * this is neccessary because the DatePicker component
 * from react-datepicker package has a property (selected)
 * that requires an input of type Date if we passed another
 * type the Component will give back an error and the app will crash
 */
export function fromConverter(from) {
  if (/^now-/.test(from)) {
    const { _from } = convertPresetsToDate(from);
    return { from: _from * 1000 };
  }
  return { from: moment(from * 1000).toDate() };
}

export function untilConverter(until) {
  if (until === "now") {
    return { until: moment().toDate() };
  }
  return { until: moment(until * 1000).toDate() };
}
