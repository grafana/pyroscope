// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck
import React from 'react';
import { Switch, Route, useRouteMatch, NavLink } from 'react-router-dom';
import Box from '@webapp/ui/Box';
import Icon from '@webapp/ui/Icon';
import { faKey } from '@fortawesome/free-solid-svg-icons/faKey';
import { faLock } from '@fortawesome/free-solid-svg-icons/faLock';
import { faSlidersH } from '@fortawesome/free-solid-svg-icons/faSlidersH';
import { faUserAlt } from '@fortawesome/free-solid-svg-icons/faUserAlt';
import cx from 'classnames';
import { useAppSelector } from '@webapp/redux/hooks';
import { selectCurrentUser } from '@webapp/redux/reducers/user';
import { User } from '@webapp/models/users';
import Preferences from './Preferences';
import Security from './Security';
import Users from './Users';
import ApiKeys from './APIKeys';

import styles from './Settings.module.css';
import UserAddForm from './Users/UserAddForm';
import APIKeyAddForm from './APIKeys/APIKeyAddForm';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import trace from '../../../traces/trace2.json';
import lodash from 'lodash';

function traceToFlamebearer(trace: any) {
  let result = {
    topLevel: 0,
    rangeMin: 0,
    format: 'single' as const,
    numTicks: 0,
    sampleRate: 1000000,
    names: [],
    levels: [],

    rangeMax: 1,
    units: 'samples',
    fitMode: 'HEAD',

    spyName: 'tracing',
  };

  // Step 1: converting spans to a tree
  var spans = {};
  var root = { children: [] };
  trace.spans.forEach((span) => {
    span.children = [];
    spans[span.spanID] = span;
  });

  trace.spans.forEach((span) => {
    let node = root;
    if (span.references && span.references.length > 0) {
      node = spans[span.references[0].spanID] || root;
    }
    node.children.push(span);
  });

  // Step 2: group spans with same name

  function groupSpans(span: any, d: int) {
    (span.children || []).forEach((x) => groupSpans(x, d + 1));

    let childrenDur = 0;
    const groups = lodash.groupBy(span.children || [], (x) => x.operationName);
    span.children = lodash.map(groups, (group) => {
      let res = group[0];
      for (let i = 1; i < group.length; i += 1) {
        res.duration += group[i].duration;
      }
      childrenDur += res.duration;
      return res;
    });
    span.total = span.duration || childrenDur;
    span.self = Math.max(0, span.total - childrenDur);
  }
  groupSpans(root, 0);

  // Step 3: traversing the tree

  function processNode(span: any, level: int, offset: int) {
    result.numTicks ||= span.total;
    result.levels[level] ||= [];
    result.levels[level].push(offset);
    result.levels[level].push(span.total);
    result.levels[level].push(span.self);
    result.names.push(
      (span.processID
        ? trace.processes[span.processID].serviceName + ': '
        : '') + (span.operationName || 'total')
    );
    result.levels[level].push(result.names.length - 1);

    (span.children || []).forEach((x) => {
      offset += processNode(x, level + 1, offset);
    });
    return span.total;
  }

  processNode(root, 0, 0);

  return result;
}

function Settings() {
  const { path, url } = useRouteMatch();
  const currentUser = useAppSelector(selectCurrentUser);

  const isAdmin = (user: User) => user && user.role === 'Admin';
  const isExternal = (user: User) => user && user.isExternal;
  return (
    <div className="pyroscope-app">
      <h1>Settings</h1>
      <nav>
        <ul className={styles.settingsNav}>
          <li>
            <NavLink
              to={`${url}`}
              exact
              className={(isActive) =>
                cx({ [styles.navLink]: true, [styles.navLinkActive]: isActive })
              }
              data-testid="settings-profiletab"
            >
              <Icon icon={faSlidersH} /> Profile
            </NavLink>
          </li>
          {!isExternal(currentUser) && (
            <>
              <li>
                <NavLink
                  to={`${url}/security`}
                  className={(isActive) =>
                    cx({
                      [styles.navLink]: true,
                      [styles.navLinkActive]: isActive,
                    })
                  }
                  data-testid="settings-changepasswordtab"
                >
                  <Icon icon={faLock} /> Change Password
                </NavLink>
              </li>
            </>
          )}
          {isAdmin(currentUser) ? (
            <>
              <li>
                <NavLink
                  to={`${url}/users`}
                  className={(isActive) =>
                    cx({
                      [styles.navLink]: true,
                      [styles.navLinkActive]: isActive,
                    })
                  }
                  data-testid="settings-userstab"
                >
                  <Icon icon={faUserAlt} /> Users
                </NavLink>
              </li>
              <li>
                <NavLink
                  to={`${url}/api-keys`}
                  className={(isActive) =>
                    cx({
                      [styles.navLink]: true,
                      [styles.navLinkActive]: isActive,
                    })
                  }
                  data-testid="settings-apikeystab"
                >
                  <Icon icon={faKey} /> API keys
                </NavLink>
              </li>
            </>
          ) : null}
        </ul>
      </nav>
      <div className="main-wrapper">
        <Box className={styles.settingsWrapper}>
          <Switch>
            <Route exact path={path}>
              <Preferences />
            </Route>
            <Route exact path={`${path}/tracing`}>
              <FlamegraphRenderer
                flamebearer={traceToFlamebearer(trace.data[0])}
              />
            </Route>
            <Route path={`${path}/security`}>
              <Security />
            </Route>
            <Route exact path={`${path}/users`}>
              <Users />
            </Route>
            <Route exact path={`${path}/users/add`}>
              <UserAddForm />
            </Route>
            <Route exact path={`${path}/api-keys`}>
              <ApiKeys />
            </Route>
            <Route exact path={`${path}/api-keys/add`}>
              <APIKeyAddForm />
            </Route>
          </Switch>
        </Box>
      </div>
    </div>
  );
}

export default Settings;
