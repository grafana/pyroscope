// There's a copy of this function in welcome.html
//   TODO: maybe dedup somehow?
function basename() {
  const baseURLMetaTag = document.querySelector(
    'meta[name="pyroscope-base-url"]'
  );
  if (!baseURLMetaTag) {
    return null;
  }

  const baseURL = baseURLMetaTag.content;

  if (!baseURL) {
    return null;
  }
  const url = new URL(baseURL, window.location.href);
  return url.pathname;
}

export default basename;
