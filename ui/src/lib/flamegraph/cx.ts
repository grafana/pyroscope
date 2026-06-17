// Tiny `cx` replacement matching the subset of @emotion/css's cx we used.
// Accepts strings (kept), falsy values (dropped), and `{ [class]: cond }`
// objects (kept when cond is truthy). Joins with a single space.
export function cx(
  ...args: Array<string | false | null | undefined | Record<string, boolean>>
): string {
  const out: string[] = [];
  for (const a of args) {
    if (!a) continue;
    if (typeof a === 'string') out.push(a);
    else for (const k in a) if (a[k]) out.push(k);
  }
  return out.join(' ');
}
