// Whether a query matches a node
export function isMatch(query: string, nodeName: string) {
  try {
    const regex = new RegExp(query, 'i');
    return regex.test(nodeName);
  } catch (e) {
    return false;
  }
}
