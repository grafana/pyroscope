import React, { useEffect, useState } from 'react';
import { Link, useHistory } from 'react-router-dom';
import Button from '@ui/Button';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  reloadUsers,
  selectUsers,
  enableUser,
  disableUser,
} from '@pyroscope/redux/reducers/settings';
import {
  selectCurrentUser,
  withCurrentUser,
} from '@pyroscope/redux/reducers/user';
import UserTableItem from './UserTableItem';

import userStyles from './Users.module.css';
import tableStyles from '../SettingsTable.module.css';

function Users() {
  const dispatch = useAppDispatch();
  const users = useAppSelector(selectUsers);
  const currentUser = useAppSelector(selectCurrentUser);
  const history = useHistory();
  const [search, setSearchField] = useState('');

  useEffect(() => {
    dispatch(reloadUsers());
  }, []);

  if (!currentUser) return null;

  const displayUsers =
    (users &&
      users.filter(
        (x) =>
          JSON.stringify(x).toLowerCase().indexOf(search.toLowerCase()) !== -1
      )) ||
    [];

  const onDisableUser = (user: User) => {
    if (user.isDisabled) {
      dispatch(enableUser(user));
    } else {
      dispatch(disableUser(user));
    }
  };

  return (
    <>
      <h2>Users</h2>
      <div className={userStyles.searchContainer}>
        <input
          type="text"
          placeholder="Search user"
          value={search}
          onChange={(v) => setSearchField(v.target.value)}
        />
        <Button
          type="submit"
          kind="secondary"
          onClick={() => history.push('/settings/user-add')}
        >
          Add
        </Button>
      </div>
      <table
        className={[userStyles.usersTable, tableStyles.settingsTable].join(' ')}
      >
        <thead>
          <tr>
            <td />
            <td>Login</td>
            <td>Email</td>
            <td>Name</td>
            <td>Role</td>
            <td>Updated</td>
            <td />
          </tr>
        </thead>
        <tbody>
          {displayUsers.length ? (
            displayUsers.map((user) => (
              <UserTableItem
                user={user}
                isCurrent={user.id === currentUser.id}
                key={`userTableItem${user.email}`}
                onDisable={() => onDisableUser(user)}
              />
            ))
          ) : (
            <tr>
              <td className={userStyles.usersTableEmptyMessage} colSpan={7}>
                The list is empty
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </>
  );
}

export default Users;
