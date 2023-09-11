import { Query } from '@pyroscope/models/query';

/**
 * takes a page name and 2 optional queries
 * handling it appropriately when they are not preset
 * and returns only the page title if no query is set
 */
export function formatTitle(
  pageName: string,
  leftQuery?: Query,
  rightQuery?: Query
) {
  const separator = ' | ';

  if (leftQuery && rightQuery) {
    return [pageName, formatTwoQueries(leftQuery, rightQuery)]
      .filter(Boolean)
      .join(separator);
  }

  if (leftQuery) {
    return [pageName, leftQuery].filter(Boolean).join(separator);
  }

  if (rightQuery) {
    return [pageName, rightQuery].filter(Boolean).join(separator);
  }

  // None of them is defined, this may happen when there's no query in the URL
  return pageName;
}

/** formatTwoQueries assumes they both are defined and non empty */
function formatTwoQueries(leftQuery: Query, rightQuery: Query) {
  const separator = ' and ';
  if (leftQuery === rightQuery) {
    return leftQuery;
  }

  return [leftQuery, rightQuery].join(separator);
}
