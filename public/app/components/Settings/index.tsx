import React from 'react';
import { Switch, Route, useRouteMatch, NavLink } from 'react-router-dom';
import Box from '@pyroscope/ui/Box';
import Icon from '@pyroscope/ui/Icon';
import { faKey } from '@fortawesome/free-solid-svg-icons/faKey';
import { faLock } from '@fortawesome/free-solid-svg-icons/faLock';
import { faSlidersH } from '@fortawesome/free-solid-svg-icons/faSlidersH';
import { faUserAlt } from '@fortawesome/free-solid-svg-icons/faUserAlt';
import { faNetworkWired } from '@fortawesome/free-solid-svg-icons/faNetworkWired';
import cx from 'classnames';
import { useAppSelector } from '@pyroscope/redux/hooks';
import { selectCurrentUser } from '@pyroscope/redux/reducers/user';
import { User } from '@pyroscope/models/users';
import PageTitle from '@pyroscope/components/PageTitle';
import Preferences from './Preferences';
import Security from './Security';
import Users from './Users';
import Apps from './Apps';
import ApiKeys from './APIKeys';

import styles from './Settings.module.css';
import UserAddForm from './Users/UserAddForm';
import APIKeyAddForm from './APIKeys/APIKeyAddForm';
import { PageContentWrapper } from '@pyroscope/pages/PageContentWrapper';

function Settings() {
  const { path, url } = useRouteMatch();
  const currentUser = useAppSelector(selectCurrentUser);

  const isAdmin = (user?: User) => user && user.role === 'Admin';
  const isExternal = (user?: User) => user && user.isExternal;

  return (
    <div className="pyroscope-app">
      <h1>Settings</h1>
      <nav>
        <ul className={styles.settingsNav}>
          <li>
            <NavLink
              to={url}
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
              <li>
                <NavLink
                  to={`${url}/apps`}
                  className={(isActive) =>
                    cx({
                      [styles.navLink]: true,
                      [styles.navLinkActive]: isActive,
                    })
                  }
                  data-testid="settings-appstab"
                >
                  <Icon icon={faNetworkWired} /> Apps
                </NavLink>
              </li>
            </>
          ) : null}
        </ul>
      </nav>
      <PageContentWrapper>
        <Box className={styles.settingsWrapper}>
          <Switch>
            <Route exact path={path}>
              <>
                <PageTitle title="Settings / Preferences" />
                <Preferences />
              </>
            </Route>
            <Route path={`${path}/security`}>
              <>
                <PageTitle title="Settings / Security" />
                <Security />
              </>
            </Route>
            <Route exact path={`${path}/users`}>
              <>
                <PageTitle title="Settings / Users" />
                <Users />
              </>
            </Route>
            <Route exact path={`${path}/users/add`}>
              <>
                <PageTitle title="Settings / Users / Add" />
                <UserAddForm />
              </>
            </Route>
            <Route exact path={`${path}/api-keys`}>
              <>
                <PageTitle title="Settings / API Keys" />
                <ApiKeys />
              </>
            </Route>
            <Route exact path={`${path}/api-keys/add`}>
              <>
                <PageTitle title="Settings / Add API Key" />
                <APIKeyAddForm />
              </>
            </Route>
            <Route exact path={`${path}/apps`}>
              <>
                <PageTitle title="Settings / Apps" />
                <Apps />
              </>
            </Route>
          </Switch>
        </Box>
      </PageContentWrapper>
    </div>
  );
}

export default Settings;
