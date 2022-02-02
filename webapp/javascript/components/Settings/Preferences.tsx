/* eslint-disable prettier/prettier */
import React, { useState } from 'react';
import Button from '@ui/Button';
import { connect } from 'react-redux';

import { useAppDispatch } from '@pyroscope/redux/hooks';

import {
  selectCurrentUser,
  changeMyPassword,
} from '@pyroscope/redux/reducers/user';
import styles from './Preferences.module.css';

function ChangePasswordForm(props) {
  const [form, setForm] = useState({ errors: [] });
  const dispatch = useAppDispatch();

  const handleChange = (e) => {
    setForm({ ...form, [e.target.name]: e.target.value });
  };

  const onChangePassword = (form) => {
    dispatch(
      changeMyPassword({
        oldPassword: form.oldPassword,
        newPassword: form.password,
      })
    )
      .unwrap()
      .then(
        () =>
          setForm({
            errors: [],
            oldPassword: '',
            password: '',
            passwordAgain: '',
          }),
        (e) => setForm({ errors: e.errors })
      );
  };

  return (
    <>
      <h3>Change password</h3>
      <div>
        <div className={styles.preferencesInputWrapper}>
          <div className={styles.errors}>{form.errors.join(', ')}</div>
          <h4>Old password</h4>
          <input
            type="password"
            placeholder="Password"
            defaultValue=""
            name="oldPassword"
            required
            onChange={handleChange}
          />
        </div>
        <div className={styles.preferencesInputWrapper}>
          <h4>New password</h4>
          <input
            type="password"
            placeholder="Password"
            defaultValue=""
            name="password"
            required
            onChange={handleChange}
          />
        </div>
        <div className={styles.preferencesInputWrapper}>
          <h4>New password Again</h4>
          <input
            type="password"
            placeholder="Password"
            name="password-again"
            required
            onChange={handleChange}
          />
        </div>
        <Button
          type="submit"
          kind="secondary"
          onClick={() => onChangePassword(form)}
        >
          Save
        </Button>
      </div>
    </>
  );
}

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

      <ChangePasswordForm user={currentUser} />
    </>
  );
}

export default connect((state) => ({ currentUser: selectCurrentUser(state) }))(
  Preferences
);
