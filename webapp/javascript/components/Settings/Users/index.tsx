import React from 'react';
import Button from '@ui/Button';
import UserTableItem from './UserTableItem';

import styles from './Users.module.css';

function Users() {
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

  return (
    <>
      <h2>Users</h2>
      <div className={styles.searchContainer}>
        <input
          type="text"
          placeholder="Search user"
          defaultValue=""
          onChange={() => {}}
        />
        <Button type="submit" kind="secondary">
          Invite
        </Button>
      </div>
      <table className={styles.usersTable}>
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
          {sampleUsers.length ? (
            sampleUsers.map((user) => (
              <UserTableItem user={user} key={`userTableItem${user.email}`} />
            ))
          ) : (
            <tr>
              <td className={styles.usersTableEmptyMessage} colSpan={7}>
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
