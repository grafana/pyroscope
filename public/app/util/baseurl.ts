/**
 basename checks the "href" value of the <base> tag if available
 and returns the pathname part
 otherwise it return undefined
 */
export function baseurl() {
  const base = document.querySelector('base') as HTMLBaseElement;
  if (!base) {
    return undefined;
  }

  const url = new URL(base.href, window.location.href);
  return url.pathname;
}

export function baseurlForAPI() {
  // When serving production, api path is one level above /ui
  // TODO(eh-am): remove when pages are moved to root
  return baseurl()?.replace('/ui', '');
}

export default baseurlForAPI;
