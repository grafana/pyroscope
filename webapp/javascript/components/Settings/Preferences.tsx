import React from 'react';
import Button from '@ui/Button';

import styles from './Preferences.module.css';

function Preferences() {
  return (
    <>
      <h2>Edit profile</h2>
      <div className={styles.preferencesInputWrapper}>
        <h4>Name</h4>
        <input
          type="text"
          placeholder="Name"
          defaultValue=""
          required
          onChange={() => {}}
        />
      </div>
      <div className={styles.preferencesInputWrapper}>
        <h4>Password</h4>
        <input
          type="password"
          placeholder="Password"
          defaultValue=""
          required
          onChange={() => {}}
        />
      </div>
      <div className={styles.preferencesInputWrapper}>
        <h4>Username</h4>
        <input
          type="text"
          placeholder="Username"
          defaultValue=""
          required
          onChange={() => {}}
        />
      </div>
      <Button type="submit" kind="secondary">
        Save
      </Button>
    </>
  );
}

export default Preferences;
