export function newRelativeTimeRange(scalar, unit) {
  const end = Date.now();
  switch (unit) {
    case 's':
      return { start: end - scalar * 1000, end };
    case 'm':
      return { start: end - scalar * 60 * 1000, end };
    case 'h':
      return { start: end - scalar * 60 * 60 * 1000, end };
    case 'd':
      return { start: end - scalar * 24 * 60 * 60 * 1000, end };
    default:
      throw new Error(`Invalid unit: ${unit}`);
  }
}
