import React, { useState } from 'react';
import Button from '@ui/Button';

import { useAppDispatch } from '@pyroscope/redux/hooks';

import { changeMyPassword } from '@pyroscope/redux/reducers/user';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import StatusMessage from '@ui/StatusMessage';
import styles from './Security.module.css';

function ChangePasswordForm(props) {
  const { user } = props;
  const [form, setForm] = useState({ errors: [] });
  const dispatch = useAppDispatch();
  if (user.isExternal) {
    return null;
  }

  const handleChange = (e) => {
    setForm({ ...form, [e.target.name]: e.target.value });
  };

  const handleFormSubmit = (evt) => {
    evt.preventDefault();
    if (form.password !== form.passwordAgain) {
      return setForm({ errors: ['Passwords must match'] });
    }
    dispatch(
      changeMyPassword({
        oldPassword: form.oldPassword,
        newPassword: form.password,
      })
    )
      .unwrap()
      .then(
        () => {
          dispatch(
            addNotification({
              type: 'success',
              title: 'Password changed',
              message: `Password has been successfully changed`,
            })
          );
          setForm({
            errors: [],
            oldPassword: '',
            password: '',
            passwordAgain: '',
          });
        },
        (e) => setForm({ errors: e.errors })
      );
    return false;
  };

  return (
    <>
      <h2>Change password</h2>
      <div>
        <form onSubmit={handleFormSubmit}>
          <StatusMessage type="error">{form.errors.join(', ')}</StatusMessage>
          <div className={styles.securityInputWrapper}>
            <h4>Old password</h4>
            <input
              type="password"
              placeholder="Password"
              name="oldPassword"
              required
              onChange={handleChange}
            />
          </div>
          <div className={styles.securityInputWrapper}>
            <h4>New password</h4>
            <input
              type="password"
              placeholder="New password"
              name="password"
              required
              onChange={handleChange}
            />
          </div>
          <div className={styles.securityInputWrapper}>
            <h4>Confirm new password</h4>
            <input
              type="password"
              placeholder="New password"
              name="passwordAgain"
              required
              onChange={handleChange}
            />
          </div>
          <Button type="submit" kind="secondary">
            Save
          </Button>
        </form>
      </div>
    </>
  );
}

export default ChangePasswordForm;
