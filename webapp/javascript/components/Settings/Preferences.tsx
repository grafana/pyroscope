import React from 'react';
import Button from '@ui/Button';
import { connect } from 'react-redux';

import { useAppSelector } from '@pyroscope/redux/hooks';

import { selectCurrentUser } from '@pyroscope/redux/reducers/user';
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

export default connect((state) => ({ currentUser: selectCurrentUser(state) }))(
  Preferences
);
