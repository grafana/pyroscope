// Whether a query matches a node
export function isMatch(query: string, nodeName: string) {
  const regex = new RegExp(query, 'i');
  return regex.test(nodeName);
}
