import React from 'react';
import { Switch, Route, useRouteMatch, NavLink } from 'react-router-dom';
import Box from '@ui/Box';
import Icon from '@ui/Icon';
import {
  faKey,
  faLock,
  faSlidersH,
  faUserAlt,
} from '@fortawesome/free-solid-svg-icons';
import cx from 'classnames';
import { withCurrentUser } from '@pyroscope/redux/reducers/user';
import Preferences from './Preferences';
import Security from './Security';
import Users from './Users';
import ApiKeys from './APIKeys';

import styles from './Settings.module.css';
import UserAddForm from './Users/UserAddForm';
import APIKeyAddForm from './APIKeys/APIKeyAddForm';

function Settings(props) {
  const { path, url } = useRouteMatch();
  const { currentUser } = props;
  const isAdmin = (user) => user && user.role === 'Admin';
  const isExternal = (user) => user && user.isExternal;
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
            >
              <Icon icon={faSlidersH} /> Profile
            </NavLink>
          </li>
          {!isExternal(currentUser) && (
            <li>
              <NavLink
                to={`${url}/security`}
                className={(isActive) =>
                  cx({
                    [styles.navLink]: true,
                    [styles.navLinkActive]: isActive,
                  })
                }
              >
                <Icon icon={faLock} /> Change Password
              </NavLink>
            </li>
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
            <Route exact path={`${path}/security`}>
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

export default withCurrentUser(Settings);
