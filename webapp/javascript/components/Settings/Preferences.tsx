import React from 'react';
import Button from '@ui/Button';

import styles from './Preferences.module.css';

function Preferences() {
  return (
    <>
      <h2>Edit profile</h2>

      <div className={styles.preferencesInputWrapper}>
        <h4>Username</h4>
        <input
          type="text"
          placeholder="Name"
          defaultValue=""
          required
          onChange={() => {}}
        />
      </div>

      <div className={styles.preferencesInputWrapper}>
        <h4>Full Name</h4>
        <input
          type="text"
          placeholder="Username"
          defaultValue=""
          required
          onChange={() => {}}
        />
      </div>

      <div className={styles.preferencesInputWrapper}>
        <h4>Email</h4>
        <input
          type="text"
          placeholder="email"
          defaultValue=""
          required
          onChange={() => {}}
        />
      </div>
      <Button type="submit" kind="secondary">
        Save
      </Button>

      <h3>Change password</h3>
      <div>
        <div className={styles.preferencesInputWrapper}>
          <h4>Current password</h4>
          <input
            type="password"
            placeholder="Password"
            defaultValue=""
            required
            onChange={() => {}}
          />
        </div>
        <div className={styles.preferencesInputWrapper}>
          <h4>New password</h4>
          <input
            type="password"
            placeholder="Password"
            defaultValue=""
            required
            onChange={() => {}}
          />
        </div>
        <div className={styles.preferencesInputWrapper}>
          <h4>New password Again</h4>
          <input
            type="password"
            placeholder="Password"
            defaultValue=""
            required
            onChange={() => {}}
          />
        </div>
        <Button type="submit" kind="secondary">
          Save
        </Button>
      </div>
    </>
  );
}

export default Preferences;
