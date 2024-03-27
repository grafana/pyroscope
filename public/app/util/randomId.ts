/**
 * generates a random ID to be used outside react-land (eg jquery)
 * it's mandatory to generate it once, preferably on a function's body
 * IMPORTANT: it does NOT:
 * - generate unique ids across server/client
 *   use `useId` instead (https://reactjs.org/docs/hooks-reference.html#useid)
 * - guarantee no collisions will happen (although it's unlikely)
 */
export function randomId(prefix?: string) {
  const letters = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ';
  const num = 5;

  const id = Array(num)
    .fill(0)
    .map(() => letters.substr(Math.floor(Math.random() * num + 1), 1))
    .join('');

  if (prefix) {
    return `${prefix}-${id}`;
  }

  return id;
}
