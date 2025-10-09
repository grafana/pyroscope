export function appendLabelToQuery(
  query: string,
  label: string,
  labelValue: string
) {
  // Check if label is a "legacy" label name (i.e. only
  // contains characters in [a-zA-Z0-9_]). If not legacy,
  // need to wrap the label name in quotes
  const legacyLabelRegex = /^[a-zA-Z0-9_]+$/;
  if (!legacyLabelRegex.test(label)) {
    label = `"${label}"`;
  }

  const case1Regexp = new RegExp(`${label}=.+?(\\}|,)`);
  if (query.match(case1Regexp)) {
    return query.replace(case1Regexp, `${label}="${labelValue}"$1`);
  }
  if (query.indexOf('{}') !== -1) {
    return query.replace('}', `${label}="${labelValue}"}`);
  }
  if (query.indexOf('}') !== -1) {
    return query.replace('}', `, ${label}="${labelValue}"}`);
  }

  console.warn('TODO: handle this case');
  return query;
}
