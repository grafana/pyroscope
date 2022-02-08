import React, { useState } from 'react';
import Button from '@ui/Button';

import { useAppDispatch } from '@pyroscope/redux/hooks';

import { changeMyPassword } from '@pyroscope/redux/reducers/user';
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

  const onChangePassword = (form) => {
    dispatch(
      changeMyPassword({
        oldPassword: form.oldPassword,
        newPassword: form.password,
      })
    )
      .unwrap()
      .then(
        () => {
          setForm({
            errors: [],
            oldPassword: '',
            password: '',
            passwordAgain: '',
          });
        },
        (e) => setForm({ errors: e.errors })
      );
  };

  return (
    <>
      <h2>Change password</h2>
      <div>
        <div className={styles.errors}>{form.errors.join(', ')}</div>
        <div className={styles.securityInputWrapper}>
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
        <div className={styles.securityInputWrapper}>
          <h4>New password</h4>
          <input
            type="password"
            placeholder="New password"
            defaultValue=""
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

export default ChangePasswordForm;
