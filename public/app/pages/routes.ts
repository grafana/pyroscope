export enum ROUTES {
  SINGLE_VIEW = '/',
}

// isRouteActive detects whether a route is active
// Notice that it does exact matches, so subpaths may not work correctly
export function isRouteActive(pathname: string, route: ROUTES) {
  return pathname === route;
}
