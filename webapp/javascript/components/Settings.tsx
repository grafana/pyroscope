import React from 'react';
import Box from '@ui/Box';
import Button from '@ui/Button';

import styles from './Settings.module.css';

function Settings() {
  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Box className={styles.settingsWrapper}>
          <h1>Edit profile</h1>
          <div className={styles.settingsInputWrapper}>
            <h4>Name</h4>
            <input
              type="text"
              placeholder="Name"
              defaultValue=""
              required
              onChange={() => {}}
            />
          </div>
          <div className={styles.settingsInputWrapper}>
            <h4>Password</h4>
            <input
              type="password"
              placeholder="Password"
              defaultValue=""
              required
              onChange={() => {}}
            />
          </div>
          <div className={styles.settingsInputWrapper}>
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
        </Box>
      </div>
    </div>
  );
}

export default Settings;
