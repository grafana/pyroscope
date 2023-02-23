function SearchRegex(pattern: string) {
  return {
    regex: new RegExp(pattern, 'i'),
    pattern,
  };
}

// Whether a query matches a node
// It caches the RegExp object since the query most likely won't change between calls
// Since it's the same node
export const isMatch = (() => {
  let regex = SearchRegex('noop');

  return (query: string, nodeName: string) => {
    try {
      if (regex.pattern !== query) {
        regex = SearchRegex(query);
      }
      return regex.regex.test(nodeName);
    } catch (e) {
      return false;
    }
  };
})();
