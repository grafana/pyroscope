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

function convertPresetsToDate(from) {
  const { groups } = from.match(/^now-(?<number>\d+)(?<multiplier>\D+)$/);
  const { number, multiplier } = groups;
  let _multiplier = multiplierMapping[multiplier];
  if (number > 1) {
    _multiplier += "s";
  }
  // convert preset to a Date object
  const fromObjectForm = moment().add(-number, _multiplier).toDate();

  return { fromObjectForm, number, _multiplier };
}

function formateDate(from, until, preset = true) {
  const { fromObjectForm, number, _multiplier } = preset
    ? convertPresetsToDate(from)
    : {};

  /**
   * this default values are assinged depending on the state of the parameters from and until
   */

  const _from = Math.round(moment(fromObjectForm || from * 1000));
  const _until =
    until !== "now" ? Math.round(moment(until * 1000)) : Math.round(moment());

  let readableDateForm = {
    range: `Last ${number} ${_multiplier}`,
    from: _from,
    until: _until,
  };

  if (until === "now") {
    // check if from is not a preset
    if (!/^now-/.test(from)) {
      readableDateForm = {
        ...readableDateForm,
        from: _from,
        range: `${moment(_from).format("lll")} - now`,
      };
      return readableDateForm;
    }
    return readableDateForm;
  }

  readableDateForm = {
    ...readableDateForm,
    range: `${moment(_from).format("lll")} - ${moment(_until).format("llll")}`,
    until: _until,
  };

  return readableDateForm;
}

export default function humanReadableRange(until, from) {
  if (/^now-/.test(from)) {
    return formateDate(from, until);
  }

  return formateDate(from, until, false);
  // return from + " to " +until;
}
