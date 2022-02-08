/* eslint-disable prettier/prettier */
import React from 'react';
import Button from '@ui/Button';

import { withCurrentUser } from '@pyroscope/redux/reducers/user';
import styles from './Preferences.module.css';

function Preferences(props) {
  const { currentUser } = props;

  if (!currentUser) return <></>;

  return (
    <>
      <h2>Edit profile</h2>

      <div className={styles.preferencesInputWrapper}>
        <h4>Username</h4>
        <input
          type="text"
          placeholder="username"
          value={currentUser.name}
          required
          disabled={currentUser.isExternal}
          onChange={() => {}}
        />
      </div>

      <div className={styles.preferencesInputWrapper}>
        <h4>Full Name</h4>
        <input
          type="text"
          placeholder="Full Name"
          value={currentUser.fullName}
          required
          disabled={currentUser.isExternal}
          onChange={() => {}}
        />
      </div>

      <div className={styles.preferencesInputWrapper}>
        <h4>Email</h4>
        <input
          type="text"
          placeholder="email"
          value={currentUser.email}
          required
          disabled={currentUser.isExternal}
          onChange={() => {}}
        />
      </div>
      <Button type="submit" kind="secondary">
        Save
      </Button>
    </>
  );
}

export default withCurrentUser(Preferences);
