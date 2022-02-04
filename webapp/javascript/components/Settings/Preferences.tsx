/* eslint-disable prettier/prettier */
import React, { useState } from 'react';
import Button from '@ui/Button';
import { connect } from 'react-redux';

import { useAppDispatch } from '@pyroscope/redux/hooks';

import {
  changeMyPassword,
  editMe,
  withCurrentUser,
} from '@pyroscope/redux/reducers/user';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import styles from './Preferences.module.css';

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

function Preferences(props) {
  const { currentUser } = props;
  const dispatch = useAppDispatch();

  const [form, setForm] = useState(currentUser);
  const handleFormSubmit = (evt) => {
    evt.preventDefault();

    dispatch(editMe(form))
      .unwrap()
      .then(() => {
        dispatch(
          addNotification({
            type: 'success',
            title: 'Success',
            message: 'User has been successfully edited',
          })
        );
      });
  };

  const handleFormChange = (event) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  const isEditDisabled = !!(currentUser && currentUser.isExternal);

  if (!currentUser) return <></>;
  return (
    <>
      <h2>Edit profile</h2>
      <form onSubmit={handleFormSubmit}>
        <div className={styles.preferencesInputWrapper}>
          <h4>Username</h4>
          <input
            type="text"
            placeholder="username"
            value={form?.name}
            name="name"
            required
            disabled={isEditDisabled}
            onChange={handleFormChange}
          />
        </div>

        <div className={styles.preferencesInputWrapper}>
          <h4>Full Name</h4>
          <input
            type="text"
            placeholder="Full Name"
            name="fullName"
            value={form?.fullName}
            required
            disabled={isEditDisabled}
            onChange={handleFormChange}
          />
        </div>

        <div className={styles.preferencesInputWrapper}>
          <h4>Email</h4>
          <input
            type="text"
            placeholder="email"
            value={form?.email}
            required
            name="email"
            disabled={isEditDisabled}
            onChange={handleFormChange}
          />
        </div>
        <Button type="submit" kind="secondary">
          Save
        </Button>
      </form>

      <ChangePasswordForm user={currentUser} />
    </>
  );
}

export default withCurrentUser(Preferences);
