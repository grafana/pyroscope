import { createBrowserHistory } from 'history';
import { baseurl } from '@webapp/util/baseurl';

// We share the same instance since react-query-sync expects the same
// 'history' instance to be shared
// https://github.com/Treora/redux-query-sync/blob/2a2d08e92b2bf931196f97fdbffb0c5ccfb9b6c9/Readme.md?plain=1#L126
export const history = createBrowserHistory({
  basename: baseurl(),
});
