import React, { useEffect, useState } from 'react';
import { useHistory } from 'react-router-dom';
import cl from 'classnames';
import { faPlus } from '@fortawesome/free-solid-svg-icons/faPlus';

import Button from '@pyroscope/ui/Button';
import TableUI from '@pyroscope/ui/Table';
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
import { type User } from '@pyroscope/models/users';
import Input from '@pyroscope/ui/Input';
import { getUserTableRows } from './getUserTableRows';

import userStyles from './Users.module.css';
import tableStyles from '../SettingsTable.module.scss';

const headRow = [
  { name: '', label: '', sortable: 0 },
  { name: '', label: 'Username', sortable: 0 },
  { name: '', label: 'Email', sortable: 0 },
  { name: '', label: 'Name', sortable: 0 },
  { name: '', label: 'Role', sortable: 0 },
  { name: '', label: 'Updated', sortable: 0 },
  { name: '', label: '', sortable: 0 },
];

function Users() {
  const dispatch = useAppDispatch();
  const users = useAppSelector(selectUsers);
  const currentUser = useAppSelector(selectCurrentUser);
  const history = useHistory();
  const [search, setSearchField] = useState('');

  useEffect(() => {
    dispatch(reloadUsers());
  }, [dispatch]);
  const displayUsers =
    (users &&
      users.filter(
        (x) =>
          JSON.stringify(x).toLowerCase().indexOf(search.toLowerCase()) !== -1
      )) ||
    [];

  if (!currentUser) {
    return null;
  }

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
              title: 'User has been enabled',
              message: `User id#${user.id} has been disabled`,
            })
          )
        );
    }
  };

  const handleDeleteUser = (user: User) => {
    dispatch(deleteUser(user))
      .unwrap()
      .then(() => {
        dispatch(
          addNotification({
            type: 'success',
            title: 'User has been deleted',
            message: `User id#${user.id} has been successfully deleted`,
          })
        );
      });
  };

  const tableBodyProps =
    displayUsers.length > 0
      ? {
          bodyRows: getUserTableRows(
            currentUser.id,
            displayUsers,
            handleDisableUser,
            handleDeleteUser
          ),
          type: 'filled' as const,
        }
      : {
          type: 'not-filled' as const,
          value: 'The list is empty',
          bodyClassName: userStyles.usersTableEmptyMessage,
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
          data-testid="settings-adduser"
        >
          Add User
        </Button>
      </div>
      <div className={userStyles.searchContainer}>
        <Input
          type="text"
          placeholder="Search user"
          value={search}
          onChange={(v) => setSearchField(v.target.value)}
          name="Search user input"
        />
      </div>
      <TableUI
        className={cl(userStyles.usersTable, tableStyles.settingsTable)}
        table={{ headRow, ...tableBodyProps }}
      />
    </>
  );
}

export default Users;
