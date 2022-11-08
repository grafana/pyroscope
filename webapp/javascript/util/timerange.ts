/**
 * Create an interval that's the largest possible, given a list of intervals
 */
export function createBiggestInterval({
  from,
  until,
}: {
  from: number[];
  until: number[];
}) {
  return {
    from: Math.min(...from),
    until: Math.max(...until),
  };
}
