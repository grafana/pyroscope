export const withDisplayName = (name) => (o) =>
  Object.assign(o, { displayName: name });

export default {
  withDisplayName,
};
