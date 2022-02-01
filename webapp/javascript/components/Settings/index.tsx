import React from 'react';
import { Switch, Route, useRouteMatch, NavLink } from 'react-router-dom';
import Box from '@ui/Box';
import Icon from '@ui/Icon';
import {
  faKey,
  faSlidersH,
  faUserAlt,
} from '@fortawesome/free-solid-svg-icons';
import Preferences from './Preferences';
import Users from './Users';
import ApiKeys from './ApiKeys';

import styles from './Settings.module.css';
import UserAddForm from './Users/UserAddForm';

function Settings() {
  const { path, url } = useRouteMatch();

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
                styles.navLink + (isActive ? ` ${styles.navLinkActive}` : '')
              }
            >
              <Icon icon={faSlidersH} /> Profile
            </NavLink>
          </li>
          <li>
            <NavLink
              to={`${url}/users`}
              exact
              className={(isActive) =>
                styles.navLink + (isActive ? ` ${styles.navLinkActive}` : '')
              }
            >
              <Icon icon={faUserAlt} /> Users
            </NavLink>
          </li>
          <li>
            <NavLink
              to={`${url}/api-keys`}
              exact
              className={(isActive) =>
                styles.navLink + (isActive ? ` ${styles.navLinkActive}` : '')
              }
            >
              <Icon icon={faKey} /> API keys
            </NavLink>
          </li>
        </ul>
      </nav>
      <div className="main-wrapper">
        <Box className={styles.settingsWrapper}>
          <Switch>
            <Route exact path={path}>
              <Preferences />
            </Route>
            <Route exact path={`${path}/users`}>
              <Users />
            </Route>
            <Route exact path={`${path}/user-add`}>
              <UserAddForm />
            </Route>
            <Route exact path={`${path}/api-keys`}>
              <ApiKeys />
            </Route>
          </Switch>
        </Box>
      </div>
    </div>
  );
}

export default Settings;
