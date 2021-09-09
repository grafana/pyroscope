export function isAbortError(err) {
  if (!err) {
    return false;
  }

  // https://developer.mozilla.org/en-US/docs/Web/API/DOMException
  return err.name === 'AbortError'
    || err.code === 20;
}
