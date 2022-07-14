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

import f1 from '../../../../cypress/fixtures/simple-golang-app-cpu.json';
import f2 from '../../../../cypress/fixtures/simple-golang-app-cpu2.json';

function flamebearersToTree(f1: Flamebearer, f2: Flamebearer) {
  const lookup = {};
  let root;
  [f1, f2].forEach((f, fi) => {
    for (let i = 0; i < f.levels.length; i += 1) {
      for (let j = 0; j < f.levels[i].length; j += 4) {
        const name = f.names[f.levels[i][j + 3]];
        const key = i + name;
        lookup[key] ||= { name: name, children: [], self: [], total: [] };
        const obj = lookup[key];
        obj.total[fi] = f.levels[i][j + 1];
        obj.self[fi] = f.levels[i][j + 2];
        const offset = f.levels[i][j + 0];
        if (i === 0) {
          root = obj;
        } else {
          for (let k = 0; k < f.levels[i - 1].length; k += 4) {
            const parentName = f.names[f.levels[i - 1][k + 3]];
            const parentKey = i - 1 + parentName;
            const parentOffset = f.levels[i - 1][k + 0];
            const total = f.levels[i - 1][k + 1];
            if (offset >= parentOffset && offset < parentOffset + total) {
              lookup[parentKey].children.push(obj);
              break;
            }
          }
        }
      }
    }
  });

  return root;
}

function diffFlamebearer(f1: Flamebearer, f2: Flamebearer): Flamebearer {
  const result: Flamebearer = {
    format: 'double',
    numTicks: f1.numTicks + f2.numTicks,
    leftTicks: f1.numTicks,
    rightTicks: f2.numTicks,
    maxSelf: 100,
    sampleRate: 1000000,
    names: [],
    levels: [],
    units: f1.units,
    spyName: f1.spyName,
  };

  const tree = flamebearersToTree(f1, f2);
  const processNode = (node, level, offsetLeft, offsetRight) => {
    const { name, children, self, total } = node;
    result.names.push(name);
    result.levels[level] ||= [];
    result.maxSelf = Math.max(result.maxSelf, self[0] || 0, self[1] || 0);
    result.levels[level] = result.levels[level].concat([
      offsetLeft,
      total[0] || 0,
      self[0] || 0,
      offsetRight,
      total[1] || 0,
      self[1] || 0,
      result.names.length - 1,
    ]);
    for (let i = 0; i < children.length; i += 1) {
      const [ol, or] = processNode(
        children[i],
        level + 1,
        offsetLeft,
        offsetRight
      );
      offsetLeft += ol;
      offsetRight += or;
    }
    return [total[0] || 0, total[1] || 0];
  };

  processNode(tree, 0, 0, 0);

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
            <Route exact path={`${path}/diff`}>
              <FlamegraphRenderer
                flamebearer={diffFlamebearer(f1.flamebearer, f2.flamebearer)}
                onlyDisplay="flamegraph"
                viewType="double"
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
