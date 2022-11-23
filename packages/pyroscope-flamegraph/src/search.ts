// Whether a query matches a node
export function isMatch(query: string, nodeName: string) {
  return nodeName.toLowerCase().includes(query.toLowerCase());
}
