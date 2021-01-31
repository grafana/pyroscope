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

export default function humanReadableRange(until, from) {
  if (until === "now" && typeof from === "string") {
    const m = from.match(/^now-(?<number>\d+)(?<multiplier>\D+)$/);
    if (m && multiplierMapping[m.groups.multiplier]) {
      let multiplier = multiplierMapping[m.groups.multiplier];
      if (m.groups.number > 1) {
        multiplier += "s";
      }
      const readableDateForm = {
        range: `Last ${m.groups.number} ${multiplier}`,
        from: moment().add(-m.groups.number, multiplier).toDate(),
        until: moment().toDate(),
      };
      return readableDateForm;
    }
  }
  const readableDateForm = {
    range: `${moment(from * 1000).format("lll")} – ${
      until === "now"
        ? moment().format("lll")
        : moment(until * 1000).format("lll")
    }`,
    from: moment(from * 1000).toDate(),
    until: until === "now" ? moment().toDate() : moment(until * 1000).toDate(),
  };

  return readableDateForm;
  // return from + " to " +until;
}
