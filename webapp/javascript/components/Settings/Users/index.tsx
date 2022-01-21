import React, { useState } from 'react';
import Button from '@ui/Button';
import UserTableItem from './UserTableItem';

import userStyles from './Users.module.css';
import tableStyles from '../SettingsTable.module.css';

const sampleUsers = [
  {
    login: 'SampleUserLogin',
    name: 'Test User Name',
    email: 'sampleUserEmail@gmail.com',
    lastActive: 'never',
  },
  {
    login: 'SampleUser2Login',
    name: 'Test User 2 Name',
    email: 'sampleUser2Email@gmail.com',
    lastActive: 'yesterday',
  },
];

function Users() {
  const [search, setSearchField] = useState('');
  const users = sampleUsers.filter(
    (x) => JSON.stringify(x).toLowerCase().indexOf(search.toLowerCase()) !== -1
  );
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
        <Button type="submit" kind="secondary">
          Invite
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
            <td>Seen</td>
            <td>Role</td>
            <td />
          </tr>
        </thead>
        <tbody>
          {users.length ? (
            users.map((user) => (
              <UserTableItem user={user} key={`userTableItem${user.email}`} />
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
