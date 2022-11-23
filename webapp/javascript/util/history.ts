// src/myHistory.js
import { createBrowserHistory } from 'history';
import basename from './baseurl';

const history = createBrowserHistory({
  basename: basename(),
});
export default history;
