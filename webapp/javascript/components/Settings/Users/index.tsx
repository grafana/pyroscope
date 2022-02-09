import React, { useEffect, useState } from 'react';
import { useHistory } from 'react-router-dom';

import { faPlus } from '@fortawesome/free-solid-svg-icons';

import Button from '@ui/Button';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  reloadUsers,
  selectUsers,
  enableUser,
  disableUser,
  deleteUser,
} from '@pyroscope/redux/reducers/settings';
import { selectCurrentUser } from '@pyroscope/redux/reducers/user';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import { type User } from '@models/users';
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

  const handleDisableUser = (user: User) => {
    if (user.isDisabled) {
      dispatch(enableUser(user))
        .unwrap()
        .then(() =>
          dispatch(
            addNotification({
              type: 'success',
              title: 'User has been enabled',
              message: `User id#${user.id} has been enabled`,
            })
          )
        );
    } else {
      dispatch(disableUser(user))
        .unwrap()
        .then(() =>
          dispatch(
            addNotification({
              type: 'success',
              title: 'User has been disabled',
              message: `User id#${user.id} has been disabled`,
            })
          )
        );
    }
  };

  const handleDeleteUser = (user: User) => {
    dispatch(deleteUser(user))
      .unwrap()
      .then((d) => {
        dispatch(
          addNotification({
            type: 'success',
            title: 'User has been deleted',
            message: `User id#${user.id} has been successfully deleted`,
          })
        );
      });
  };

  return (
    <>
      <h2>Users</h2>
      <div className={userStyles.actionContainer}>
        <Button
          type="submit"
          kind="secondary"
          icon={faPlus}
          onClick={() => history.push('/settings/users/add')}
        >
          Add User
        </Button>
      </div>
      <div className={userStyles.searchContainer}>
        <input
          type="text"
          placeholder="Search user"
          value={search}
          onChange={(v) => setSearchField(v.target.value)}
        />
      </div>
      <table
        className={[userStyles.usersTable, tableStyles.settingsTable].join(' ')}
      >
        <thead>
          <tr>
            <td />
            <th>Login</th>
            <th>Email</th>
            <th>Name</th>
            <th>Role</th>
            <th>Updated</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {displayUsers.length ? (
            displayUsers.map((user) => (
              <UserTableItem
                user={user}
                isCurrent={user.id === currentUser.id}
                key={`userTableItem${user.id}`}
                onDisable={handleDisableUser}
                onDelete={handleDeleteUser}
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
