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

import {
  FlamegraphRenderer,
  convertPprofToProfile,
} from '@pyroscope/flamegraph/src';

function base64ToArrayBuffer(base64: string) {
  var binaryString = window.atob(base64);
  var len = binaryString.length;
  var bytes = new Uint8Array(len);
  for (var i = 0; i < len; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  return bytes;
}

const pprofBuf = base64ToArrayBuffer(`
SKCe2fq3l7+8FgoECAEQAgoECAMQBFCPvs2MBFoECAMQBGCAreIEIhEIARjPk6+DECIFCAEQgAMQASoICAEQBRgFIAYiEAgCGPOtpoMQIgQIAhA/EAEqCAgCEAcYByAIIhEIAxjf/ZyDECIFCAMQtQEQASoICAMQCRgJIAoiEQgEGPe6qIMQIgUIBBC8ChABKggIBBALGAsgDCIRCAUY7+uogxAiBQgFENEREAEqCAgFEA0YDSAMIhEIBhiHl6mDECIFCAYQ5BYQASoICAYQDhgOIAwiEQgHGNu+qYMQIgUIBxC1GBABKggIBxAPGA8gDCIRCAgY18ipgxAiBQgIEMoZEAEqCAgIEBAYECAMIhEICRj3hrSDECIFCAkQuQEQASoICAkQERgRIBISEhAQEIDQpUwKCQECAwQFBgcICSIcCAoYsPGjhBAiBAgKEA0iBAgLEBciBAgMECYQASoICAoQExgTIBQqCAgLEBUYFSAUKggIDBAWGBYgFCIRCAsY6+GngxAiBQgNEOEBEAEqCAgNEBcYFyAMEgsQARCAreIECAoICyIRCAwY74+vgxAiBQgOEN0CEAEqCAgOEBgYGCAGIhAIDRj3paaDECIECA8QfxABKggIDxAZGBkgGiIRCA4Yl6mpgxAiBQgGEK0UEAESDxAWEIDe82gKBgwNDgcICSIRCA8Yj5WvgxAiBQgQEI4DEAEqCAgQEBsYGyAGIhAIEBjPsKaDECIECBEQSRABKggIERAcGBwgCCIRCBEYi/ucgxAiBQgSEKEBEAEqCAgSEB0YHSAKIhEIEhir8KiDECIFCBMQsRIQASoICBMQHhgeIAwiEQgTGLP8qIMQIgUIFBCBExABKggIFBAfGB8gDCIRCBQY97GpgxAiBQgVEKAXEAEqCAgVECAYICAMIhEIFRjDvqmDECIFCAcQvBgQARISEAMQgIenDgoJDxAREhMUFQgJIhEIFhiPhq+DECIFCBYQ6QEQASoICBYQIRghIAYiFggXGJejpoMQIgQIFxAeIgQIGBBZEAEqCAgXECIYIiAjKggIGBAkGCQgGiIRCBgYu5epgxAiBQgGEOEWEAESDxABEICt4gQKBhYXGAcICSIRCBkYj7GpgxAiBQgZEI4XEAEqCAgZECUYJSAMIhEIGhjDuK+DECIFCBoQ7AMQASoICBoQJhgmICciGAgbGPear4MQIgUIGxCgBCIFCBwQygEQASoICBsQKBgoICcqCAgcECkYKSAnIhEIHBj7yKmDECIFCAgQvxkQARITEAEQgK3iBAoKDxAREhMZGhscCSIRCB0Y55CpgxAiBQgGEL8WEAESDxADEICHpw4KBgwNHQcICRoECAE4ATIAMgdzYW1wbGVzMgVjb3VudDIDY3B1MgtuYW5vc2Vjb25kczIZcnVudGltZS5wdGhyZWFkX2NvbmRfd2FpdDJAL29wdC9ob21lYnJldy9DZWxsYXIvZ28vMS4xNi4xL2xpYmV4ZWMvc3JjL3J1bnRpbWUvc3lzX2Rhcndpbi5nbzIRcnVudGltZS5zZW1hc2xlZXAyPy9vcHQvaG9tZWJyZXcvQ2VsbGFyL2dvLzEuMTYuMS9saWJleGVjL3NyYy9ydW50aW1lL29zX2Rhcndpbi5nbzIRcnVudGltZS5ub3Rlc2xlZXAyPy9vcHQvaG9tZWJyZXcvQ2VsbGFyL2dvLzEuMTYuMS9saWJleGVjL3NyYy9ydW50aW1lL2xvY2tfc2VtYS5nbzINcnVudGltZS5tUGFyazI6L29wdC9ob21lYnJldy9DZWxsYXIvZ28vMS4xNi4xL2xpYmV4ZWMvc3JjL3J1bnRpbWUvcHJvYy5nbzINcnVudGltZS5zdG9wbTIUcnVudGltZS5maW5kcnVubmFibGUyEHJ1bnRpbWUuc2NoZWR1bGUyDnJ1bnRpbWUucGFya19tMg1ydW50aW1lLm1jYWxsMj4vb3B0L2hvbWVicmV3L0NlbGxhci9nby8xLjE2LjEvbGliZXhlYy9zcmMvcnVudGltZS9hc21fYXJtNjQuczIJbWFpbi53b3JrMjYvVXNlcnMvZG1pdHJ5L0Rldi9wcy9weXJvc2NvcGUvZXhhbXBsZXMvZ29sYW5nL21haW4uZ28yEW1haW4uc2xvd0Z1bmN0aW9uMgltYWluLm1haW4yDHJ1bnRpbWUubWFpbjIOcnVudGltZS5rZXZlbnQyD3J1bnRpbWUubmV0cG9sbDJEL29wdC9ob21lYnJldy9DZWxsYXIvZ28vMS4xNi4xL2xpYmV4ZWMvc3JjL3J1bnRpbWUvbmV0cG9sbF9rcXVldWUuZ28yG3J1bnRpbWUucHRocmVhZF9jb25kX3NpZ25hbDIScnVudGltZS5zZW1hd2FrZXVwMhJydW50aW1lLm5vdGV3YWtldXAyDnJ1bnRpbWUuc3RhcnRtMg1ydW50aW1lLndha2VwMhVydW50aW1lLnJlc2V0c3Bpbm5pbmcyDnJ1bnRpbWUud3JpdGUxMg1ydW50aW1lLndyaXRlMkEvb3B0L2hvbWVicmV3L0NlbGxhci9nby8xLjE2LjEvbGliZXhlYy9zcmMvcnVudGltZS90aW1lX25vZmFrZS5nbzIUcnVudGltZS5uZXRwb2xsQnJlYWsyFXJ1bnRpbWUud2FrZU5ldFBvbGxlcjIQcnVudGltZS5tb2R0aW1lcjI6L29wdC9ob21lYnJldy9DZWxsYXIvZ28vMS4xNi4xL2xpYmV4ZWMvc3JjL3J1bnRpbWUvdGltZS5nbzIScnVudGltZS5yZXNldHRpbWVyMhVydW50aW1lLnJlc2V0Rm9yU2xlZXA=

`);

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
            <Route path={`${path}/security`}>
              <Security />
            </Route>
            <Route exact path={`${path}/pprof`}>
              <FlamegraphRenderer
                profile={convertPprofToProfile(pprofBuf, 'samples')}
                ExportData={null}
              />
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
