import React, { useState } from 'react';
import Button from '@ui/Button';

import { useAppDispatch } from '@pyroscope/redux/hooks';

import { changeMyPassword } from '@pyroscope/redux/reducers/user';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import StatusMessage from '@ui/StatusMessage';
import InputField from '@ui/InputField';

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
          <StatusMessage type="error" message={form.errors.join(', ')} />
          <InputField
            label="Old password"
            type="password"
            placeholder="Password"
            name="oldPassword"
            required
            onChange={handleChange}
          />
          <InputField
            label="New password"
            type="password"
            placeholder="New password"
            name="password"
            required
            onChange={handleChange}
          />
          <InputField
            label="Confirm new password"
            type="password"
            placeholder="New password"
            name="passwordAgain"
            required
            onChange={handleChange}
          />
          <Button type="submit" kind="secondary">
            Save
          </Button>
        </form>
      </div>
    </>
  );
}

export default ChangePasswordForm;
